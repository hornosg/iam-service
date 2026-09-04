# onboarding

Módulo **vacío por diseño** (ACC-E01 T4). Ningún archivo actual del servicio vive acá:
llenarlo dentro de E01 es scope creep (riesgo registrado en la épica).

Se llena en **ACC-E05** (saga de onboarding genérica). Hasta entonces, este directorio
existe sólo para fijar la frontera del módulo que el test de arquitectura de T6
(`src/arch_test.go`) va a proteger: `identity` y `access` no pueden importar
`src/onboarding`.