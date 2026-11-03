"""Exact compatibility lookup. A Linux guest never proves desktop qualification."""
from .contracts import environment


def compatibility(recipe, observed, operation):
    environment(observed)
    if observed["enforcement"] != "verified":
        return {"eligible": False, "reason": "resource/storage enforcement is not verified"}
    for item in recipe["environments"]:
        if item["environment"] == observed and operation in item["operations"]:
            return {"eligible": True, "reason": "exact signed environment and operation match",
                    "qualificationDigest": item["qualificationDigest"]}
    return {"eligible": False, "reason": "this environment/version/operation has no matching qualification"}
