#!/usr/bin/env python3
"""Contrato DOCUMENTAL de los dos hallazgos ACC-E04.T1, no test DB ni de logs.

Los bloques JSON normativos son la superficie comprobable: se parsean sólo en
su subsección de DD-1, con claves únicas y valores exactos. Una palabra suelta,
un ejemplo fuera del contrato o un bloque en DD-2 no satisfacen el checker.
Sólo stdlib; --self-test muta copias temporales, jamás el ADR real.
"""
import argparse
import json
from pathlib import Path
import re
import tempfile


BOOT_HEADER = "### Bootstrap atómico (T3) — contrato normativo"
AUDIT_HEADER = "### Auditoría cross-tenant (T4) — contrato normativo"
BOOT = {
    "operation": "bootstrap.insert_missing",
    "primitive": "INSERT INTO tenant_config",
    "conflict_clause": "ON CONFLICT (tenant_id, config_key) DO NOTHING",
    "separate_from_admin_save": True,
    "pre_read_required": False,
    "transaction": "all defaults in one WithRLSInTransaction; SET LOCAL app.tenant_id; SQL through tx only",
    "tenant_source": "validated non-nil UUID after tenant:provision gate",
    "preserve_on_conflict": ["config_value", "id", "created_at", "updated_at"],
    "created_count": "actual inserted rows after commit only; conflict=0; rollback=0 with error",
    "repeat": "existing rows unchanged; created_count=0",
}
AUDIT = {
    "event": "tenant.config_access",
    "emitter": "tenancy Infrastructure route audit wrapper",
    "wiring": "router composition -> required canonical adapter -> wrapper before RequireScope -> controller -> final emit after outcome",
    "adapter": "go-shared v0.17.0 CanonicalLogger.Emit",
    "sink": "JSON stdout -> Docker logs -> existing Promtail -> Loki",
    "envelope": ["ts", "level", "service", "event"],
    "fields": ["s2s_service", "tenant_id", "operation", "outcome", "correlation_id"],
    "identity_source": "Authorize s2s_service context only; otherwise unattributed; never caller headers",
    "tenant_source": "validated non-nil target UUID after gate; otherwise omitted",
    "operation": ["config.read", "config.bootstrap"],
    "outcome": ["success", "not_found", "denied", "invalid_request", "error"],
    "correlation_source": "server-generated UUID; never raw caller header",
    "forbidden": ["API key", "Authorization", "config value", "raw error", "raw headers", "PII"],
    "emission": "one local attempt per completed in-process request, including gate denials and failures",
    "sink_failure": "best-effort fail-open for audit only; never bypass authorization or RLS; no retry or rollback for logging",
    "guarantee": "Emit returns void and ignores marshal/write errors; no persistence or remote delivery acknowledgement",
    "limits": "stdout may block or lose events; no durable queue, remote durability, exactly-once, or Loki delivery proof",
}


def section(text, header, level):
    """Un único heading literal fuera de fences, hasta el siguiente mismo/superior."""
    lines = text.splitlines(keepends=True)
    starts = []
    fence = None
    headings = []
    offset = 0
    for line in lines:
        stripped = line.strip()
        if stripped.startswith(("```", "~~~")):
            delimiter = stripped[:3]
            if fence is None:
                fence = delimiter
            elif delimiter == fence:
                fence = None
        elif fence is None:
            match = re.match(r"^(#{1,6}) ", line)
            if match:
                headings.append((offset, len(match[1])))
                if line.rstrip("\r\n") == header:
                    starts.append(offset)
        offset += len(line)
    if len(starts) != 1:
        raise ValueError(f"se exige una única sección {header!r}; encontradas {len(starts)}")
    start = starts[0]
    end = next((pos for pos, depth in headings if pos > start and depth <= level), len(text))
    return text[start:end]


def unique_object(pairs):
    obj = {}
    for key, value in pairs:
        if key in obj:
            raise ValueError(f"clave JSON duplicada: {key}")
        obj[key] = value
    return obj


def contract(text, header):
    dd1_headers = re.findall(r"^## DD-1[^\n]*", text, re.M)
    if len(dd1_headers) != 1:
        raise ValueError("DD-1 ausente o duplicado")
    dd1 = section(text, dd1_headers[0], 2)
    scoped = section(dd1, header, 3)
    blocks = re.findall(r"^```json\s*\n(.*?)^```\s*$", scoped, re.M | re.S)
    if len(blocks) != 1:
        raise ValueError(f"{header}: se exige exactamente un bloque JSON normativo")
    return json.loads(blocks[0], object_pairs_hook=unique_object)


def validate(text):
    for header, expected in ((BOOT_HEADER, BOOT), (AUDIT_HEADER, AUDIT)):
        actual = contract(text, header)
        # JSON canónico distingue booleanos de enteros (True == 1 en Python).
        if json.dumps(actual, sort_keys=True) != json.dumps(expected, sort_keys=True):
            raise ValueError(f"{header}: contrato divergente, incompleto o con claves extra")


def replace_contract(text, header, transform):
    scoped = section(text, header, 3)
    block = re.search(r"^```json\s*\n(.*?)^```\s*$", scoped, re.M | re.S)
    obj = json.loads(block[1])
    transform(obj)
    changed = scoped[:block.start(1)] + json.dumps(obj, ensure_ascii=False, indent=2) + "\n" + scoped[block.end(1):]
    return text.replace(scoped, changed, 1)


def self_test(text):
    validate(text)
    print("PASS documental: contrato vigente")
    mutants = {}
    for header, expected in ((BOOT_HEADER, BOOT), (AUDIT_HEADER, AUDIT)):
        for key in expected:
            mutants[f"quitar {header.split()[1]}.{key}"] = replace_contract(text, header, lambda o, k=key: o.pop(k))
    mutants["eliminar DO NOTHING"] = replace_contract(text, BOOT_HEADER, lambda o: o.update(conflict_clause=BOOT["conflict_clause"].replace(" DO NOTHING", "")))
    mutants["introducir DO UPDATE"] = replace_contract(text, BOOT_HEADER, lambda o: o.update(conflict_clause="ON CONFLICT (tenant_id, config_key) DO UPDATE SET config_value = EXCLUDED.config_value"))
    for field in AUDIT["fields"]:
        mutants[f"quitar campo audit {field}"] = replace_contract(text, AUDIT_HEADER, lambda o, f=field: o["fields"].remove(f))
    for field in BOOT["preserve_on_conflict"]:
        mutants[f"no preservar {field}"] = replace_contract(text, BOOT_HEADER, lambda o, f=field: o["preserve_on_conflict"].remove(f))
    mutants["identidad desde header"] = replace_contract(text, AUDIT_HEADER, lambda o: o.update(identity_source="X-Service-Name header"))
    mutants["durabilidad remota inventada"] = replace_contract(text, AUDIT_HEADER, lambda o: o.update(guarantee="durable delivery to Loki"))
    mutants["wrapper después del gate"] = replace_contract(text, AUDIT_HEADER, lambda o: o.update(wiring=AUDIT["wiring"].replace("before RequireScope", "after RequireScope")))
    mutants["reusar Save administrativo"] = replace_contract(text, BOOT_HEADER, lambda o: o.update(separate_from_admin_save=False))
    mutants["contador antes de commit"] = replace_contract(text, BOOT_HEADER, lambda o: o.update(created_count="number of defaults attempted"))
    mutants["booleano como entero"] = replace_contract(text, BOOT_HEADER, lambda o: o.update(separate_from_admin_save=1))
    for header in (BOOT_HEADER, AUDIT_HEADER):
        scoped = section(text, header, 3)
        mutants[f"contrato fuera de DD-1: {header}"] = text.replace(scoped, "", 1) + "\n## Apéndice señuelo\n" + scoped
        mutants[f"contrato sólo en fence: {header}"] = text.replace(scoped, "```markdown\n" + scoped + "```\n", 1)
        mutants[f"contrato duplicado: {header}"] = text.replace(scoped, scoped + scoped, 1)
    mutants["clave JSON duplicada"] = text.replace('"separate_from_admin_save": true,', '"separate_from_admin_save": false,\n  "separate_from_admin_save": true,', 1)
    # Copia sin enmienda, conservando palabras correctas como señuelo fuera de DD-1.
    prior = text
    for header in (BOOT_HEADER, AUDIT_HEADER):
        prior = prior.replace(section(prior, header, 3), "", 1)
    mutants["sin enmienda con palabras señuelo"] = prior + "\n## Notas\nDO NOTHING s2s_service tenant_id Loki CanonicalLogger\n"
    with tempfile.TemporaryDirectory(prefix="adr005-negative-") as tmp:
        for i, (name, mutant) in enumerate(mutants.items()):
            path = Path(tmp) / f"negative-{i}.md"
            path.write_text(mutant, encoding="utf-8")
            try:
                validate(path.read_text(encoding="utf-8"))
            except (ValueError, TypeError):
                print(f"PASS negativo: {name}")
            else:
                raise ValueError(f"mutante aceptado indebidamente: {name}")
    print(f"PASS: {len(mutants)} negativos rechazados; sólo contrato documental, no concurrencia/emisión runtime")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--self-test", action="store_true")
    parser.add_argument("adr", type=Path)
    args = parser.parse_args()
    try:
        text = args.adr.read_text(encoding="utf-8")
        if args.self_test:
            self_test(text)
        else:
            validate(text)
            print("PASS documental: bootstrap insert-only + auditoría con garantías explícitas")
    except (OSError, ValueError, TypeError) as exc:
        parser.exit(1, f"FAIL contrato ADR-005: {exc}\n")


if __name__ == "__main__":
    main()
