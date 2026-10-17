package usecase_test

import (
	"context"
	"github.com/google/uuid"
	sharedmetrics "github.com/hornosg/go-shared/infrastructure/metrics"
	"github.com/hornosg/iam-service/src/tenancy/application/request"
	"github.com/hornosg/iam-service/src/tenancy/application/usecase"
	"github.com/hornosg/iam-service/src/tenancy/domain/exception"
	"github.com/hornosg/iam-service/src/tenancy/domain/value_object"
	"github.com/hornosg/iam-service/test/tenancy/infrastructure/persistence/repository"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestProductNamespace_CreateAndLookup(t *testing.T) {
	repo := repository.NewMockTenantRepository()
	create := usecase.NewCreateTenantUseCase(repo, sharedmetrics.NoopRecorder{})
	lookup := usecase.NewGetTenantBySlugUseCase(repo)
	// El caso HTTP prueba por separado que el claim mc no sustituye el request.
	ctx := context.Background()
	ids := map[string]uuid.UUID{}
	for _, namespace := range []string{"", "whatsapp-agent"} {
		result, err := create.Execute(ctx, &request.CreateTenantRequest{
			Namespace: namespace, Name: "Store", Slug: "same-slug", Description: "Test store", Type: "BUSINESS", OwnerID: uuid.NewString(),
		})
		require.NoError(t, err)
		expected := namespace
		if expected == "" {
			expected = value_object.DefaultProductNamespace
		}
		require.Equal(t, expected, result.Namespace)
		require.Equal(t, expected, entityNamespace(t, repo, result.ID))
		ids[expected] = result.ID
	}
	require.NotEqual(t, ids["mercado-cercano"], ids["whatsapp-agent"])
	for namespace, id := range ids {
		got, err := lookup.Execute(ctx, namespace, "same-slug")
		require.NoError(t, err)
		require.Equal(t, id, got.ID, "lookup no debe ignorar namespace")
		require.Equal(t, namespace, got.Namespace)
	}
	legacy, err := lookup.Execute(ctx, "", "same-slug")
	require.NoError(t, err)
	require.Equal(t, ids["mercado-cercano"], legacy.ID)
	_, err = create.Execute(ctx, &request.CreateTenantRequest{Namespace: "mercado-cercano", Name: "Other", Slug: "same-slug", Description: "Test", Type: "BUSINESS", OwnerID: uuid.NewString()})
	require.ErrorIs(t, err, exception.ErrSlugAlreadyExists)
	_, err = lookup.Execute(ctx, "other-product", "same-slug")
	require.Error(t, err)
}

func entityNamespace(t *testing.T, repo *repository.MockTenantRepository, id uuid.UUID) string {
	t.Helper()
	tenant, err := repo.GetByID(context.Background(), id)
	require.NoError(t, err)
	return tenant.Namespace
}

func TestProductNamespace_RejectMCAliasBeforePersistence(t *testing.T) {
	for _, namespace := range []string{"mc", "MC", " mc "} {
		t.Run(namespace, func(t *testing.T) {
			repo := repository.NewMockTenantRepository()
			_, err := usecase.NewCreateTenantUseCase(repo, sharedmetrics.NoopRecorder{}).Execute(context.Background(), &request.CreateTenantRequest{Namespace: namespace})
			require.ErrorIs(t, err, exception.ErrInvalidNamespace)
			_, err = usecase.NewGetTenantBySlugUseCase(repo).Execute(context.Background(), namespace, "same-slug")
			require.ErrorIs(t, err, exception.ErrInvalidNamespace)
			require.Zero(t, repo.GetCallCount("ExistsBySlug"))
			require.Zero(t, repo.GetCallCount("GetBySlug"))
			require.Zero(t, repo.GetCallCount("Create"))
		})
	}
}
