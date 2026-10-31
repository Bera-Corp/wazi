"""Fail-closed candidate composition validation; no Docker or shell execution."""
from decimal import Decimal

import context_engine as engine
import validate


READINESS = {"postgresql": "postgres-authenticated", "immich-postgresql": "postgres-authenticated",
             "mysql": "mysql-authenticated", "nginx": "nginx-config-and-http",
             "immich-server": "immich-http", "cache": "cache-authenticated"}
APPLICATIONS = {"postgresql": "org.postgresql.postgresql", "mysql": "com.mysql.mysql",
                "nginx": "org.nginx.nginx", "immich-server": "app.immich.server",
                "immich-postgresql": "app.immich.postgresql", "cache": "io.valkey.valkey"}


def validate_composition(record, manifests, available, authorized_secret_refs=()):
    """Bind reviewed components, reject cycles, and sum simultaneous demand.

    manifests is a caller-authenticated catalog, indexed by canonical digest.
    This function deliberately does not infer signature trust from JSON fields.
    available is the already authorized headroom after other apps and reserves.
    """
    errors = engine._record_schema_errors("package-composition.schema.json", record)
    if errors:
        return ["composition-schema-invalid"]
    if engine.secret_findings(record):
        return ["composition-secret-pattern"]
    components = record["components"]
    by_id = {c["id"]: c for c in components}
    if len(by_id) != len(components):
        errors.append("duplicate-component")
    roles = [c["role"] for c in components]
    totals = {key: Decimal(0) for key in ("cpuCores", "ramMb", "storageMb")}
    for component in components:
        ref = component["manifestRef"]
        manifest = manifests.get(ref["digest"])
        if not manifest or validate.canonical_digest(manifest) != ref["digest"]:
            errors.append("component-manifest-unresolved")
        elif (engine._record_schema_errors("app-manifest.schema.json", manifest)
              or manifest["appId"] != ref["recordId"] or manifest["version"] != ref.get("version")
              or manifest["appId"] != APPLICATIONS[component["role"]]
              or manifest.get("reviewState") not in ("signed", "canary", "stable")):
            errors.append("component-manifest-binding")
        else:
            requirements = manifest["requirements"]
            for limit, minimum in (("cpuCores", "minCpuCores"), ("ramMb", "minRamMb"), ("storageMb", "minStorageMb")):
                if Decimal(str(component["limits"][limit])) < Decimal(str(requirements[minimum])):
                    errors.append("component-limit-below-requirement")
        if component["readiness"] != READINESS[component["role"]]:
            errors.append("component-readiness-mismatch")
        if component["role"] in ("postgresql", "mysql", "immich-postgresql", "cache") and not component["secretRefs"]:
            errors.append("authenticated-service-secret-required")
        if set(component["secretRefs"]) - set(authorized_secret_refs):
            errors.append("undeclared-secret-reference")
        for key in totals:
            totals[key] += Decimal(str(component["limits"][key]))
    visiting, visited = set(), set()

    def visit(name):
        if name not in by_id:
            errors.append("dependency-unresolved")
            return
        if name in visiting:
            errors.append("dependency-cycle")
            return
        if name in visited:
            return
        visiting.add(name)
        for dependency in by_id[name]["dependsOn"]:
            visit(dependency)
        visiting.remove(name)
        visited.add(name)

    for name in by_id:
        visit(name)
    for key, total in totals.items():
        if key not in available or total > Decimal(str(available[key])):
            errors.append("aggregate-" + key + "-exceeds-headroom")
    grouped = set()
    for group in record["recoveryGroups"]:
        if set(group["members"]) - set(by_id):
            errors.append("recovery-member-unresolved")
        if grouped.intersection(group["members"]):
            errors.append("recovery-member-duplicated")
        grouped.update(group["members"])
    if set(by_id) - grouped:
        errors.append("recovery-coverage-missing")
    if record["profile"] == "immich-no-ml":
        if roles.count("immich-server") != 1 or roles.count("immich-postgresql") != 1 or roles.count("cache") != 1 or roles.count("nginx") != 1 or len(roles) != 4:
            errors.append("immich-component-matrix")
        else:
            server = next(c for c in components if c["role"] == "immich-server")
            db = next(c for c in components if c["role"] == "immich-postgresql")
            cache = next(c for c in components if c["role"] == "cache")
            proxy = next(c for c in components if c["role"] == "nginx")
            if db["limits"]["ramMb"] < 2048:
                errors.append("immich-postgresql-memory-minimum")
            if not {db["id"], cache["id"]} <= set(server["dependsOn"]) or server["id"] not in proxy["dependsOn"]:
                errors.append("immich-readiness-order")
            if not any(g["strategy"] == "quiesced-database-and-media" and {server["id"], db["id"]} <= set(g["members"]) for g in record["recoveryGroups"]):
                errors.append("immich-coordinated-recovery-required")
    elif len(components) != 1 or roles[0] not in ("postgresql", "mysql", "nginx"):
        errors.append("standalone-component-matrix")
    return sorted(set(errors))
