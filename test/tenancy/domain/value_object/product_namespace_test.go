package value_object_test

import (
	"github.com/hornosg/iam-service/src/tenancy/domain/exception"
	vo "github.com/hornosg/iam-service/src/tenancy/domain/value_object"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestResolveProductNamespace(t *testing.T) {
	for _, tc := range []struct {
		input, want string
		invalid     bool
	}{
		{"", "mercado-cercano", false}, {"mercado-cercano", "mercado-cercano", false}, {"whatsapp-agent", "whatsapp-agent", false},
		{"mc", "", true}, {"MC", "", true}, {" mc ", "", true}, {" ", "", true}, {strings.Repeat("x", 51), "", true},
		{strings.Repeat("x", 50), strings.Repeat("x", 50), false},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, err := vo.ResolveProductNamespace(tc.input)
			if tc.invalid {
				require.ErrorIs(t, err, exception.ErrInvalidNamespace)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.want, got)
		})
	}
}
