package port

// RoguePort vive fuera de domain/ — el gate de T6 debe cazarlo en CI.
// Archivo descartable, solo para verificar el gate en un PR de prueba.
type RoguePort interface {
	DoThing() error
}
