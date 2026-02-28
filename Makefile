# ==================================================================================== #
# HELPERS
# ==================================================================================== #

## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'


# ==================================================================================== #
# QUALITY CONTROL
# ==================================================================================== #

## audit: run quality control checks
.PHONY: audit
audit: test
	go mod tidy -diff
	go mod verify
	test -z "$(shell gofmt -l .)" 
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@latest -checks=all,-ST1000,-U1000 ./...
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

## test: run all tests
.PHONY: test
test:
	go test -v -race -buildvcs ./...

## test/cover: run all tests and display coverage
.PHONY: test/cover
test/cover:
	go test -v -race -buildvcs -coverprofile=/tmp/coverage.out ./...
	go tool cover -html=/tmp/coverage.out

## upgradeable: list direct dependencies that have upgrades available
.PHONY: upgradeable
upgradeable:
	@go run github.com/oligot/go-mod-upgrade@latest

# ==================================================================================== #
# DEVELOPMENT
# ==================================================================================== #

## tidy: tidy modfiles and format .go files
.PHONY: tidy
tidy:
	go mod tidy -v
	go fmt ./...

## build: build the cmd/api application
.PHONY: build
build:
	@echo "Building sqlwarden..."
	@mkdir -p dist
	go build -ldflags="-s -w -X github.com/sqlwarden/internal/version.version=dev -X github.com/sqlwarden/internal/version.commit=$$(git rev-parse --short HEAD 2>/dev/null || echo unknown) -X github.com/sqlwarden/internal/version.date=$$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o=dist/sqlwarden ./cmd/api

## build/release: build the application for release (requires goreleaser)
.PHONY: build/release
build/release:
	@if ! command -v goreleaser &> /dev/null; then \
		echo "goreleaser is not installed. Install it with: go install github.com/goreleaser/goreleaser@latest"; \
		exit 1; \
	fi
	goreleaser build --snapshot --clean
	
## run: run the cmd/api application
.PHONY: run
run: build
	DB_LOG_QUERIES=true ./dist/sqlwarden

## run/live: run the application with reloading on file changes
.PHONY: run/live
run/live:
	go run github.com/cosmtrek/air@v1.43.0 \
		--build.cmd "make build" --build.bin "make run" --build.delay "100" \
		--build.exclude_dir "" \
		--build.include_ext "go, tpl, tmpl, html, css, scss, js, ts, sql, jpeg, jpg, gif, png, bmp, svg, webp, ico" \
		--misc.clean_on_exit "true"


# ==================================================================================== #
# SQL MIGRATIONS
# ==================================================================================== #

## migrations/new name=$1: create a new database migration for both postgres and sqlite
.PHONY: migrations/new
migrations/new:
	go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest create -seq -ext=.sql -dir=./assets/migrations_postgres ${name}
	go run -tags 'sqlite' github.com/golang-migrate/migrate/v4/cmd/migrate@latest create -seq -ext=.sql -dir=./assets/migrations_sqlite ${name}

## migrations/up: apply all up database migrations (use DB_DRIVER=postgres or DB_DRIVER=sqlite)
.PHONY: migrations/up
migrations/up:
	@if [ -z "$(DB_DSN)" ]; then \
		echo "Error: DB_DSN is required. Example: make migrations/up DB_DRIVER=sqlite DB_DSN=sqlwarden.db"; \
		exit 1; \
	fi
	@if [ -z "$(DB_DRIVER)" ]; then \
		echo "Error: DB_DRIVER is required. Use DB_DRIVER=postgres or DB_DRIVER=sqlite"; \
		exit 1; \
	fi
	@if [ "$(DB_DRIVER)" != "postgres" ] && [ "$(DB_DRIVER)" != "sqlite" ]; then \
		echo "Error: DB_DRIVER must be either 'postgres' or 'sqlite'"; \
		exit 1; \
	fi
	@if [ "$(DB_DRIVER)" = "postgres" ]; then \
		go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest -path=./assets/migrations_postgres -database="postgres://${DB_DSN}" up; \
	else \
		go run -tags 'sqlite' github.com/golang-migrate/migrate/v4/cmd/migrate@latest -path=./assets/migrations_sqlite -database="sqlite://${DB_DSN}" up; \
	fi

## migrations/down: apply all down database migrations (use DB_DRIVER=postgres or DB_DRIVER=sqlite)
.PHONY: migrations/down
migrations/down:
	@if [ -z "$(DB_DSN)" ]; then \
		echo "Error: DB_DSN is required. Example: make migrations/down DB_DRIVER=sqlite DB_DSN=sqlwarden.db"; \
		exit 1; \
	fi
	@if [ -z "$(DB_DRIVER)" ]; then \
		echo "Error: DB_DRIVER is required. Use DB_DRIVER=postgres or DB_DRIVER=sqlite"; \
		exit 1; \
	fi
	@if [ "$(DB_DRIVER)" != "postgres" ] && [ "$(DB_DRIVER)" != "sqlite" ]; then \
		echo "Error: DB_DRIVER must be either 'postgres' or 'sqlite'"; \
		exit 1; \
	fi
	@if [ "$(DB_DRIVER)" = "postgres" ]; then \
		go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest -path=./assets/migrations_postgres -database="postgres://${DB_DSN}" down; \
	else \
		go run -tags 'sqlite' github.com/golang-migrate/migrate/v4/cmd/migrate@latest -path=./assets/migrations_sqlite -database="sqlite://${DB_DSN}" down; \
	fi

## migrations/goto version=$1: migrate to a specific version number (use DB_DRIVER=postgres or DB_DRIVER=sqlite)
.PHONY: migrations/goto
migrations/goto:
	@if [ -z "$(DB_DSN)" ]; then \
		echo "Error: DB_DSN is required. Example: make migrations/goto version=1 DB_DRIVER=sqlite DB_DSN=sqlwarden.db"; \
		exit 1; \
	fi
	@if [ -z "$(DB_DRIVER)" ]; then \
		echo "Error: DB_DRIVER is required. Use DB_DRIVER=postgres or DB_DRIVER=sqlite"; \
		exit 1; \
	fi
	@if [ "$(DB_DRIVER)" != "postgres" ] && [ "$(DB_DRIVER)" != "sqlite" ]; then \
		echo "Error: DB_DRIVER must be either 'postgres' or 'sqlite'"; \
		exit 1; \
	fi
	@if [ "$(DB_DRIVER)" = "postgres" ]; then \
		go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest -path=./assets/migrations_postgres -database="postgres://${DB_DSN}" goto ${version}; \
	else \
		go run -tags 'sqlite' github.com/golang-migrate/migrate/v4/cmd/migrate@latest -path=./assets/migrations_sqlite -database="sqlite://${DB_DSN}" goto ${version}; \
	fi

## migrations/force version=$1: force database migration (use DB_DRIVER=postgres or DB_DRIVER=sqlite)
.PHONY: migrations/force
migrations/force:
	@if [ -z "$(DB_DSN)" ]; then \
		echo "Error: DB_DSN is required. Example: make migrations/force version=1 DB_DRIVER=sqlite DB_DSN=sqlwarden.db"; \
		exit 1; \
	fi
	@if [ -z "$(DB_DRIVER)" ]; then \
		echo "Error: DB_DRIVER is required. Use DB_DRIVER=postgres or DB_DRIVER=sqlite"; \
		exit 1; \
	fi
	@if [ "$(DB_DRIVER)" != "postgres" ] && [ "$(DB_DRIVER)" != "sqlite" ]; then \
		echo "Error: DB_DRIVER must be either 'postgres' or 'sqlite'"; \
		exit 1; \
	fi
	@if [ "$(DB_DRIVER)" = "postgres" ]; then \
		go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest -path=./assets/migrations_postgres -database="postgres://${DB_DSN}" force ${version}; \
	else \
		go run -tags 'sqlite' github.com/golang-migrate/migrate/v4/cmd/migrate@latest -path=./assets/migrations_sqlite -database="sqlite://${DB_DSN}" force ${version}; \
	fi

## migrations/version: print the current in-use migration version (use DB_DRIVER=postgres or DB_DRIVER=sqlite)
.PHONY: migrations/version
migrations/version:
	@if [ -z "$(DB_DSN)" ]; then \
		echo "Error: DB_DSN is required. Example: make migrations/version DB_DRIVER=sqlite DB_DSN=sqlwarden.db"; \
		exit 1; \
	fi
	@if [ -z "$(DB_DRIVER)" ]; then \
		echo "Error: DB_DRIVER is required. Use DB_DRIVER=postgres or DB_DRIVER=sqlite"; \
		exit 1; \
	fi
	@if [ "$(DB_DRIVER)" != "postgres" ] && [ "$(DB_DRIVER)" != "sqlite" ]; then \
		echo "Error: DB_DRIVER must be either 'postgres' or 'sqlite'"; \
		exit 1; \
	fi
	@if [ "$(DB_DRIVER)" = "postgres" ]; then \
		go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest -path=./assets/migrations_postgres -database="postgres://${DB_DSN}" version; \
	else \
		go run -tags 'sqlite' github.com/golang-migrate/migrate/v4/cmd/migrate@latest -path=./assets/migrations_sqlite -database="sqlite://${DB_DSN}" version; \
	fi

