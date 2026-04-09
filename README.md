# food-scaner

## Dev

```bash
cd go
docker compose -f docker-compose.dev.yml up
```

## Managing users

You can run these commands **while the server is running** — no restart needed. SQLite handles concurrent access fine for quick one-shot writes.

**List all users:**
```bash
./foodscaner --list-users
```

**Approve a user** (by their auth_id shown on the pending screen):
```bash
./foodscaner --approve 123456789
```

**Revoke access:**
```bash
./foodscaner --revoke 123456789
```

**Change daily scan limit:**
```bash
./foodscaner --set-limit 123456789 20
```

On the server the binary is at `/opt/foodscaner/foodscaner`.

## Backup

**Required on server:**
```bash
apt install sqlite3
mkdir -p /backups/foodscaner
```

**Cron — daily backup, keep 7 days** (`crontab -e`):
```
0 3 * * * sqlite3 /data/foodscaner.db ".backup /backups/foodscaner/foodscaner-$(date +\%F).db" && find /backups/foodscaner -name "*.db" -mtime +7 -delete
```

Runs at 3am every night. No downtime — SQLite's `.backup` command is safe while the service is running.

## Prod deploy

```bash
# Build linux binary
docker build --target binary --output go/bin/ -f go/Dockerfile .

# Copy to server
scp go/bin/foodscaner root@your-server:/opt/foodscaner/

# First time only — set up systemd service
cp go/foodscaner.service.example /etc/systemd/system/foodscaner.service
# edit the file with real values
systemctl enable foodscaner && systemctl start foodscaner
```
