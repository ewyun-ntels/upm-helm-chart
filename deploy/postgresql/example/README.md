# PostgreSQL Example

This example connects to PgBouncer, creates a table, inserts rows, and reads them back.

## Environment variables

- `PGHOST` default: `127.0.0.1`
- `PGPORT` default: `30432`
- `PGUSER` default: `upmdb`
- `PGPASSWORD` default: `upmdb`
- `PGDATABASE` default: `upmdb`

## Run

```bash
cd deploy/postgresql/example
go run .
```

## Example with NodePort PgBouncer

```bash
PGHOST=<node-ip> \
PGPORT=30432 \
PGUSER=upmdb \
PGPASSWORD=upmdb \
PGDATABASE=upmdb \
go run .
```
