.PHONY: validate tree

validate:
	@python3 -m json.tool schemas/execution-context-envelope.schema.json >/dev/null
	@python3 -m json.tool schemas/deployment-plan.schema.json >/dev/null
	@python3 -m json.tool schemas/app-manifest.schema.json >/dev/null
	@python3 -m json.tool context/examples/host-profile.json >/dev/null
	@python3 -m json.tool context/examples/execution-context-envelope.json >/dev/null
	@echo "JSON validation passed"

tree:
	@find . -maxdepth 4 -type f | sort
