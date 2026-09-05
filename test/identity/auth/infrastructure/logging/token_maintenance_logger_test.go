package logging_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hornosg/iam-service/src/identity/domain/port"
	authlogging "github.com/hornosg/iam-service/src/identity/infrastructure/logging"
)

func TestTokenMaintenanceLogger_CleanupCompleted_EmitsCanonicalLine(t *testing.T) {
	var buf bytes.Buffer
	logger := authlogging.NewTokenMaintenanceLoggerWithWriter("iam-test", &buf)

	logger.Log(port.TokenMaintenanceEvent{
		Event: port.EventRevocationCleanupCompleted,
		Count: 42,
	})

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line), "output must be valid JSON")

	assert.Equal(t, "token.revocation_cleanup_completed", line["event"])
	assert.Equal(t, "info", line["level"])
	assert.Equal(t, "iam-test", line["service"])
	assert.NotEmpty(t, line["ts"])
	assert.EqualValues(t, 42, line["count"])
	assert.Nil(t, line["reason"], "reason must be absent on success")
}

func TestTokenMaintenanceLogger_CleanupFailed_EmitsErrorLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := authlogging.NewTokenMaintenanceLoggerWithWriter("iam-test", &buf)

	logger.Log(port.TokenMaintenanceEvent{
		Event:  port.EventRevocationCleanupFailed,
		Reason: "connection refused",
	})

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line), "output must be valid JSON")

	assert.Equal(t, "token.revocation_cleanup_failed", line["event"])
	assert.Equal(t, "error", line["level"])
	assert.Equal(t, "iam-test", line["service"])
	assert.Equal(t, "connection refused", line["reason"])
	assert.Nil(t, line["count"], "count must be absent on failure")
}

func TestTokenMaintenanceLogger_ZeroCount_OmitsCountField(t *testing.T) {
	var buf bytes.Buffer
	logger := authlogging.NewTokenMaintenanceLoggerWithWriter("iam-test", &buf)

	// Cleanup completed but with 0 entries (count=0 → no log emitted by caller;
	// but if called directly, count field should be absent)
	logger.Log(port.TokenMaintenanceEvent{
		Event: port.EventRevocationCleanupCompleted,
		Count: 0,
	})

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	assert.Nil(t, line["count"], "count=0 must be omitted (omitempty semantics)")
}

func TestTokenMaintenanceLogger_NoCredentials_NeverLogged(t *testing.T) {
	// Verificación de seguridad: el struct TokenMaintenanceEvent no expone campos de credenciales.
	// ADR-001 / L4 (iam = auth/identidad): nunca loguear passwords, hashes, bearer tokens, secrets.
	// "token" en el nombre del evento es vocabulario de dominio (OK); lo que se verifica
	// es que ningún valor de credencial pueda filtrarse.
	var buf bytes.Buffer
	logger := authlogging.NewTokenMaintenanceLoggerWithWriter("iam-test", &buf)

	logger.Log(port.TokenMaintenanceEvent{
		Event:  port.EventRevocationCleanupFailed,
		Reason: "timeout",
	})

	output := buf.String()
	// Estos valores nunca deben aparecer como datos en el output canónico.
	assert.NotContains(t, output, "password")
	assert.NotContains(t, output, "secret")
	assert.NotContains(t, output, "hash")
	assert.NotContains(t, output, "bearer")
	// El nombre de evento sí puede contener "token" (vocabulario de dominio); validamos
	// que sea el nombre de evento esperado, no un valor de credencial.
	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	assert.Equal(t, "token.revocation_cleanup_failed", line["event"],
		"event name is domain vocabulary, not a credential value")
}
