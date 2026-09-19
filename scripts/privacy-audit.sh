#!/usr/bin/env sh
# Privacy audit: verify that the application schema stores no message, chat or
# contact data. Run against a live database:
#
#   ./scripts/privacy-audit.sh
#
# Configuration (defaults match docker-compose.yml):
#   POSTGRES_USER (default wg)
#   POSTGRES_DB   (default whatsapp_gateway)
#   COMPOSE_CMD   (default "docker compose")

set -eu

POSTGRES_USER="${POSTGRES_USER:-wg}"
POSTGRES_DB="${POSTGRES_DB:-whatsapp_gateway}"
COMPOSE_CMD="${COMPOSE_CMD:-docker compose}"
FAIL=0

run_sql() {
  $COMPOSE_CMD exec -T postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tAc "$1"
}

echo "== Application schema tables =="
run_sql "SELECT table_name FROM information_schema.tables WHERE table_schema = 'app' ORDER BY table_name"

echo
echo "== Forbidden table check =="
FORBIDDEN="messages message chats chat conversations conversation contacts contact message_history media inbox"
FOUND=""
for table in $FORBIDDEN; do
  count=$(run_sql "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'app' AND table_name = '$table'")
  if [ "$count" != "0" ]; then
    echo "FAIL: forbidden table app.$table exists"
    FAIL=1
  fi
done
[ "$FAIL" = "0" ] && echo "OK: no message/chat/contact tables in app schema"

echo
echo "== Forbidden column check =="
COLUMN_FAIL=0
for column in message_text body content caption chat_id contact_id; do
  count=$(run_sql "SELECT count(*) FROM information_schema.columns WHERE table_schema = 'app' AND column_name = '$column'")
  if [ "$count" != "0" ]; then
    echo "FAIL: forbidden column app.*.$column exists"
    COLUMN_FAIL=1
    FAIL=1
  fi
done
[ "$COLUMN_FAIL" = "0" ] && echo "OK: no message-content columns in app schema"

echo
echo "== whatsmeow protocol tables (internal, never exposed) =="
run_sql "SELECT table_name FROM information_schema.tables WHERE table_schema = 'whatsapp' ORDER BY table_name" | head -40

echo
echo "== Row counts (must show no message storage) =="
run_sql "SELECT 'app.instances', count(*) FROM app.instances UNION ALL SELECT 'app.api_keys', count(*) FROM app.api_keys"

if [ "$FAIL" != "0" ]; then
  echo
  echo "PRIVACY AUDIT FAILED"
  exit 1
fi

echo
echo "PRIVACY AUDIT PASSED"
