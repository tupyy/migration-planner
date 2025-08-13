COMPOSE_FILE := $(realpath deploy/podman/compose.yaml)

deploy-db:
	@echo "🚀 Deploy DB on podman..."
	podman rm -f planner-db || true
	podman volume rm podman_planner-db || true
	podman-compose -f $(COMPOSE_FILE) up -d planner-db
	test/scripts/wait_for_postgres.sh podman
	podman exec -it planner-db psql -c 'ALTER ROLE admin WITH SUPERUSER'
	podman exec -it planner-db createdb admin || true
	podman exec -it planner-db createdb spicedb || true
	@echo "✅ DB was deployed successfully on podman."

deploy-spicedb:
	@echo "Deploy spicedb on podman..."
	podman kube play --network podman_planner-network $(CURDIR)/deploy/spicedb/kube.yml

kill-spicedb:
	@echo "Remove spicedb..."
	podman kube down $(CURDIR)/deploy/spicedb/kube.yml

kill-db:
	@echo "🗑️ Remove DB instance from podman..."
	podman-compose -f $(COMPOSE_FILE) down planner-db
	@echo "✅ DB instance was removed successfully from podman."

.PHONY: deploy-db kill-db
