# ~/coding/telegramBot/run.sh
#!/bin/sh
set -a
. "$HOME/coding/telegramBot/.env.local"
set +a
exec "$HOME/coding/telegramBot/bot"
