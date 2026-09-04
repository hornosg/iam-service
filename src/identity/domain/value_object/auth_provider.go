package value_object

type AuthProvider string

const (
	LocalAuth  AuthProvider = "LOCAL"
	GoogleAuth AuthProvider = "GOOGLE"
)

func (ap AuthProvider) IsValid() bool {
	return ap == LocalAuth || ap == GoogleAuth
}

func (ap AuthProvider) String() string {
	return string(ap)
}
