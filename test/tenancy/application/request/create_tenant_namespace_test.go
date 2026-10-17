package request_test

import (
	"encoding/json"
	"github.com/hornosg/iam-service/src/tenancy/application/request"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCreateTenantRequest_NamespaceOptionalAndDomainNormalization(t *testing.T) {
	for _, tc := range []struct{ body, namespace string }{{`{"name":"Store"}`, ""}, {`{"namespace":"whatsapp-agent"}`, "whatsapp-agent"}} {
		var got request.CreateTenantRequest
		require.NoError(t, json.Unmarshal([]byte(tc.body), &got))
		require.Equal(t, tc.namespace, got.Namespace)
		require.Empty(t, got.GetNormalizedDomain())
		got.Domain = " Store.Example.COM "
		require.Equal(t, "store.example.com", got.GetNormalizedDomain())
	}
}

func TestTenantUpdate_DomainNormalizationPreservesOptionality(t *testing.T) {
	update := request.UpdateTenantRequest{}
	require.Nil(t, update.GetNormalizedDomain())
	empty := ""
	update.Domain = &empty
	require.Nil(t, update.GetNormalizedDomain())
	domain := " Store.Example.COM "
	update.Domain = &domain
	require.Equal(t, "store.example.com", *update.GetNormalizedDomain())
}
