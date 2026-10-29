.PHONY: validate contracts tree

validate:
	@for f in schemas/*.json schemas/experimental/*.json context/examples/*.json context/worlds/*.json evals/fixtures/*.json evals/adversarial/*.json; do \
		python3 -m json.tool "$$f" >/dev/null || exit 1; \
	done
	@echo "JSON validation passed"

contracts:
	@python3 scripts/validate.py
	@python3 scripts/test_adversarial.py
	@python3 scripts/test_sprints.py

tree:
	@find . -maxdepth 4 -type f | sort
