package value_object

import (
	"github.com/hornosg/iam-service/src/tenancy/domain/exception"
	"strings"
	"unicode/utf8"
)

const DefaultProductNamespace = "mercado-cercano"

// ResolveProductNamespace aplica el default legado al request, nunca al claim JWT.
// La longitud corresponde al VARCHAR(50) existente; mc es el alias de token, no producto.
func ResolveProductNamespace(namespace string) (string, error) {
	if namespace == "" {
		return DefaultProductNamespace, nil
	}
	if strings.EqualFold(strings.TrimSpace(namespace), "mc") || strings.TrimSpace(namespace) == "" || utf8.RuneCountInString(namespace) > 50 {
		return "", exception.ErrInvalidNamespace
	}
	return namespace, nil
}
