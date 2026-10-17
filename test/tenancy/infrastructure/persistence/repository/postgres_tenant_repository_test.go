package repository_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/hornosg/go-shared/criteria"
	"github.com/hornosg/iam-service/src/tenancy/domain/entity"
	"github.com/hornosg/iam-service/src/tenancy/domain/exception"
	"github.com/hornosg/iam-service/src/tenancy/domain/port"
	vo "github.com/hornosg/iam-service/src/tenancy/domain/value_object"
	"github.com/hornosg/iam-service/src/tenancy/infrastructure/persistence/repository"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// TEST-ACC-E04-T9-SQL: every projection using shared scanners must carry namespace.
// sqlmock verifies shape/parameters and error mapping; integration test proves UNIQUE/RLS.
var tenantColumns = []string{"id", "name", "slug", "description", "type", "status", "owner_id", "domain", "plan_id", "user_count", "max_users", "settings", "features", "expires_at", "created_at", "updated_at", "namespace"}

func tenantRow() []driver.Value {
	now := time.Now()
	return []driver.Value{uuid.NewString(), "Store", "same", "Description", "BUSINESS", "ACTIVE", uuid.NewString(), "store.example.com", uuid.NewString(), 0, 10, []byte(`{"color":"blue"}`), []byte(`{"friends_family":true}`), now, now, now, "whatsapp-agent"}
}
func tenantRepo(t *testing.T) (port.TenantCriteriaRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, m.ExpectationsWereMet()); _ = db.Close() })
	return repository.NewPostgresTenantRepository(db), m
}
func TestTenantRepository_NamespaceEveryProjection(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	one := map[string]func(port.TenantCriteriaRepository) (*entity.Tenant, error){
		"id": func(r port.TenantCriteriaRepository) (*entity.Tenant, error) { return r.GetByID(ctx, id) },
		"slug": func(r port.TenantCriteriaRepository) (*entity.Tenant, error) {
			return r.GetBySlug(ctx, "whatsapp-agent", "same")
		},
		"domain": func(r port.TenantCriteriaRepository) (*entity.Tenant, error) {
			return r.GetByDomain(ctx, "store.example.com")
		},
	}
	for name, call := range one {
		t.Run(name, func(t *testing.T) {
			repo, m := tenantRepo(t)
			q := m.ExpectQuery(`SELECT .*updated_at, namespace\s+FROM tenants`)
			if name == "slug" {
				q.WithArgs("whatsapp-agent", "same")
			}
			q.WillReturnRows(sqlmock.NewRows(tenantColumns).AddRow(tenantRow()...))
			got, err := call(repo)
			require.NoError(t, err)
			require.Equal(t, "whatsapp-agent", got.Namespace)
			require.NotNil(t, got.PlanID)
			require.NotNil(t, got.ExpiresAt)
			require.Equal(t, "store.example.com", got.Domain)
			require.Equal(t, "blue", got.Settings["color"])
		})
	}
	many := map[string]func(port.TenantCriteriaRepository) ([]*entity.Tenant, error){
		"list":  func(r port.TenantCriteriaRepository) ([]*entity.Tenant, error) { return r.List(ctx, 10, 0) },
		"owner": func(r port.TenantCriteriaRepository) ([]*entity.Tenant, error) { return r.GetByOwner(ctx, id) },
		"status": func(r port.TenantCriteriaRepository) ([]*entity.Tenant, error) {
			return r.GetByStatus(ctx, vo.TenantStatusActive, 10, 0)
		},
		"type-paged": func(r port.TenantCriteriaRepository) ([]*entity.Tenant, error) {
			return r.GetByType(ctx, vo.TenantTypeBusiness, 10, 0)
		},
		"type-all": func(r port.TenantCriteriaRepository) ([]*entity.Tenant, error) {
			return r.GetByType(ctx, vo.TenantTypeBusiness, -1, 0)
		},
		"plan":     func(r port.TenantCriteriaRepository) ([]*entity.Tenant, error) { return r.GetByPlan(ctx, id, 10, 0) },
		"active":   func(r port.TenantCriteriaRepository) ([]*entity.Tenant, error) { return r.GetActive(ctx, 10, 0) },
		"expiring": func(r port.TenantCriteriaRepository) ([]*entity.Tenant, error) { return r.GetExpiring(ctx, 3) },
		"criteria": func(r port.TenantCriteriaRepository) ([]*entity.Tenant, error) {
			return r.SearchByCriteria(ctx, criteria.NewCriteriaBuilder().Build())
		},
	}
	for name, call := range many {
		for _, fail := range []bool{false, true} {
			label := "ok"
			if fail {
				label = "query-error"
			}
			t.Run(name+"/"+label, func(t *testing.T) {
				repo, m := tenantRepo(t)
				query := m.ExpectQuery(`SELECT .*updated_at, namespace\s+FROM tenants`)
				if fail {
					query.WillReturnError(errors.New("database unavailable"))
				} else {
					query.WillReturnRows(sqlmock.NewRows(tenantColumns).AddRow(tenantRow()...))
				}
				got, err := call(repo)
				if fail {
					require.ErrorContains(t, err, "database unavailable")
					require.Nil(t, got)
				} else {
					require.NoError(t, err)
					require.Len(t, got, 1)
					require.Equal(t, "whatsapp-agent", got[0].Namespace)
					require.Equal(t, "blue", got[0].Settings["color"])
				}
			})
		}
	}
}
func TestTenantRepository_RejectInvalidRows(t *testing.T) {
	for _, many := range []bool{false, true} {
		for _, bad := range []struct {
			name   string
			column int
			value  driver.Value
		}{
			{"type", 4, "INVALID"}, {"status", 5, "INVALID"}, {"settings", 11, []byte("{")}, {"features", 12, []byte("{")}, {"scan", 0, "invalid-uuid"},
		} {
			name := bad.name
			if many {
				name += "-list"
			}
			t.Run(name, func(t *testing.T) {
				repo, m := tenantRepo(t)
				row := tenantRow()
				row[bad.column] = bad.value
				m.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(tenantColumns).AddRow(row...))
				var err error
				if many {
					_, err = repo.List(context.Background(), 10, 0)
				} else {
					_, err = repo.GetByID(context.Background(), uuid.New())
				}
				require.Error(t, err)
			})
		}
	}
	t.Run("not-found", func(t *testing.T) {
		repo, m := tenantRepo(t)
		m.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(tenantColumns))
		_, err := repo.GetBySlug(context.Background(), "whatsapp-agent", "absent")
		require.ErrorIs(t, err, exception.ErrTenantNotFound)
	})
	t.Run("nullable", func(t *testing.T) {
		repo, m := tenantRepo(t)
		row := tenantRow()
		for _, i := range []int{7, 8, 11, 12, 13} {
			row[i] = nil
		}
		m.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(tenantColumns).AddRow(row...))
		got, err := repo.GetByID(context.Background(), uuid.New())
		require.NoError(t, err)
		require.Nil(t, got.PlanID)
		require.Empty(t, got.Domain)
		require.Equal(t, "whatsapp-agent", got.Namespace)
	})
	t.Run("row-error", func(t *testing.T) {
		repo, m := tenantRepo(t)
		m.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(tenantColumns).AddRow(tenantRow()...).RowError(0, errors.New("stream interrupted")))
		_, err := repo.List(context.Background(), 10, 0)
		require.ErrorContains(t, err, "stream interrupted")
	})
}
func TestTenantRepository_CreateConstraintMapping(t *testing.T) {
	for _, tc := range []struct {
		name  string
		dbErr error
		want  error
	}{
		{"ok", nil, nil}, {"slug", &pq.Error{Code: "23505", Constraint: "tenants_namespace_slug_unique"}, exception.ErrSlugAlreadyExists},
		{"domain", &pq.Error{Code: "23505", Constraint: "tenants_domain_key"}, exception.ErrDomainAlreadyExists},
		{"other-constraint", &pq.Error{Code: "23505", Constraint: "tenants_pkey"}, nil}, {"other-code", &pq.Error{Code: "23503"}, nil}, {"db-error", errors.New("unavailable"), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, m := tenantRepo(t)
			tenant := entity.NewTenant("Store", "same", "Description", vo.TenantTypeBusiness, uuid.New())
			tenant.Namespace = "whatsapp-agent"
			expectRLS(m, tenant.ID)
			insert := m.ExpectExec(`INSERT INTO tenants .*updated_at, namespace\s*\).*\$17`).WithArgs(tenant.ID, tenant.Name, tenant.Slug, tenant.Description, "BUSINESS", "ACTIVE", tenant.OwnerID, sqlmock.AnyArg(), nil, 0, tenant.MaxUsers, sqlmock.AnyArg(), sqlmock.AnyArg(), nil, tenant.CreatedAt, tenant.UpdatedAt, "whatsapp-agent")
			if tc.dbErr == nil {
				insert.WillReturnResult(sqlmock.NewResult(0, 1))
				m.ExpectCommit()
			} else {
				insert.WillReturnError(tc.dbErr)
				m.ExpectRollback()
			}
			err := repo.Create(context.Background(), tenant)
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
			} else if tc.dbErr != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
	t.Run("unserializable-settings", func(t *testing.T) {
		repo, m := tenantRepo(t)
		tenant := entity.NewTenant("Store", "s", "d", vo.TenantTypeBusiness, uuid.New())
		tenant.Settings["bad"] = make(chan int)
		expectRLS(m, tenant.ID)
		m.ExpectRollback()
		require.ErrorContains(t, repo.Create(context.Background(), tenant), "marshaling settings")
	})
}
func TestTenantRepository_PrechecksAndCounts(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	for _, found := range []bool{false, true} {
		t.Run(map[bool]string{true: "found", false: "absent"}[found], func(t *testing.T) {
			repo, m := tenantRepo(t)
			for _, slug := range []bool{true, false} {
				q := m.ExpectQuery(`SELECT 1 FROM tenants WHERE `)
				if slug {
					q.WithArgs("whatsapp-agent", "same")
				} else {
					q.WithArgs("example.com")
				}
				rows := sqlmock.NewRows([]string{"one"})
				if found {
					rows.AddRow(1)
				}
				q.WillReturnRows(rows)
				var got bool
				var err error
				if slug {
					got, err = repo.ExistsBySlug(ctx, "whatsapp-agent", "same")
				} else {
					got, err = repo.ExistsByDomain(ctx, "example.com")
				}
				require.NoError(t, err)
				require.Equal(t, found, got)
			}
		})
	}
	counts := map[string]func(port.TenantCriteriaRepository) (int, error){
		"all": func(r port.TenantCriteriaRepository) (int, error) { return r.Count(ctx) }, "owner": func(r port.TenantCriteriaRepository) (int, error) { return r.CountByOwner(ctx, id) }, "status": func(r port.TenantCriteriaRepository) (int, error) { return r.CountByStatus(ctx, vo.TenantStatusActive) }, "plan": func(r port.TenantCriteriaRepository) (int, error) { return r.CountByPlan(ctx, id) }, "criteria": func(r port.TenantCriteriaRepository) (int, error) {
			return r.CountByCriteria(ctx, criteria.NewCriteriaBuilder().Build())
		},
	}
	for name, call := range counts {
		for _, fail := range []bool{false, true} {
			label := name
			if fail {
				label += "-error"
			}
			t.Run(label, func(t *testing.T) {
				repo, m := tenantRepo(t)
				q := m.ExpectQuery(`SELECT COUNT\(\*\) FROM tenants`)
				if fail {
					q.WillReturnError(errors.New("unavailable"))
				} else {
					q.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
				}
				got, err := call(repo)
				if fail {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					require.Equal(t, 2, got)
				}
			})
		}
	}
}
