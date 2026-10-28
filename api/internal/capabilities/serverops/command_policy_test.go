package serverops

import (
	"errors"
	"testing"
)

func TestClassifyCommandProvesReadOnlyCommands(t *testing.T) {
	for _, command := range []string{
		"df -h / && free -m",
		"systemctl status nginx --no-pager",
		"systemctl --failed",
		"systemctl list-units --type=service --state=running",
		"journalctl -u nginx --since '-10 min' --no-pager | tail -n 50",
		"journalctl -xe -n 100",
		"ls -la /var/www; cat /etc/nginx/nginx.conf",
		"tail -n 200 /var/log/syslog 2>&1 | grep -i error",
		"grep -rn 'listen' /etc/nginx 2>/dev/null || true",
		"find /var/log -name '*.log' -size +100M",
		"ps aux --sort=-%mem | head -n 20",
		"ss -tlnp",
		"git -C /srv/app status && git -C /srv/app log --oneline -n 5",
		"docker ps -a && docker logs --tail 100 web",
		"docker compose -f /srv/app/docker-compose.yml ps",
		"curl -sS -m 5 http://127.0.0.1:8080/health",
		"curl -I localhost:3000",
		"sed -n '1,40p' /etc/hosts",
		"awk '{print $1}' /var/log/nginx/access.log | sort | uniq -c | sort -rn | head",
		"wc -l < /var/log/syslog",
		"/usr/bin/uptime\n",
		"du -sh /var/* 2>/dev/null | sort -h\nnproc",
		"crontab -l",
		"nginx -t",
		"ip addr show",
		"dmesg -T | tail -n 30",
		"journalctl -u app &&\n tail -n 5 /var/log/app.log",
		"echo \"done\";",
	} {
		if got := ClassifyCommand(command); got != CommandReadOnly {
			t.Errorf("ClassifyCommand(%q) = %s, want read_only", command, got)
		}
	}
}

func TestClassifyCommandTreatsEverythingElseAsAChange(t *testing.T) {
	for _, command := range []string{
		"systemctl restart nginx",
		"sudo systemctl status nginx",
		"tail -f /var/log/syslog",
		"tail -fn100 /var/log/syslog",
		"journalctl --vacuum-size=100M",
		"find /tmp -name '*.tmp' -delete",
		"find / -exec rm {} \\;",
		"echo hi > /tmp/x",
		"echo hi >> /tmp/x",
		"cat /etc/hosts > /dev/sda",
		"ls $HOME",
		"echo `id`",
		"ls $(pwd)",
		"echo \"$PATH\"",
		"(cd /srv && ls)",
		"{ ls; }",
		"sleep 10 &",
		"ls &&",
		"ls |",
		"ls && ; ls",
		"git pull",
		"git -c core.pager=x log",
		"git log --output=/tmp/x",
		"docker restart web",
		"docker exec web ls",
		"docker logs -f web",
		"curl -X POST http://127.0.0.1/api",
		"curl -o /tmp/x http://127.0.0.1/",
		"curl https://example.com/",
		"curl http://user@127.0.0.1/",
		"sed -i 's/a/b/' /etc/hosts",
		"sed 's/a/b/' /etc/hosts",
		"awk 'BEGIN{system(\"id\")}'",
		"awk '{print > \"/tmp/x\"}' f",
		"sort -o /etc/passwd /tmp/x",
		"uniq a b",
		"crontab -r",
		"hostname evil",
		"date -s '2020-01-01'",
		"cat /etc/shadow",
		"cat /srv/app/.env",
		"grep KEY < /srv/app/.env",
		"cat ~/.ssh/id_ed25519",
		"cat /proc/1/environ",
		"cat /etc/ssl/private/server.key",
		"PATH=/tmp ls",
		"./deploy.sh",
		"/tmp/ls",
		"python3 -c 'print(1)'",
		"vim /etc/hosts",
		"# comment",
		"! ls",
		"",
		"   ",
		"ls 'unterminated",
	} {
		if got := ClassifyCommand(command); got != CommandChange {
			t.Errorf("ClassifyCommand(%q) = %s, want change", command, got)
		}
	}
}

func TestClassifyCommandForbidsDestroyingTheServer(t *testing.T) {
	for _, command := range []string{
		"rm -rf /",
		"rm -rf /*",
		"rm -fr /etc",
		"rm -r --no-preserve-root /",
		"sudo rm -rf /var/",
		"cd /srv && rm -Rf ~",
		"mkfs.ext4 /dev/vdb1",
		"sudo -u root mkfs /dev/sdb",
		"dd if=/dev/zero of=/dev/vda bs=1M",
		"shutdown -h now",
		"reboot",
		"sudo poweroff",
		"systemctl reboot",
		"init 0",
		"wipefs -a /dev/sdb",
		"ls; rm -rf / $(date)",
		"x=`reboot`",
		"echo x && $(reboot)",
		"rm -rf / $X",
		":(){ :|:& };:",
	} {
		if got := ClassifyCommand(command); got != CommandForbidden {
			t.Errorf("ClassifyCommand(%q) = %s, want forbidden", command, got)
		}
	}
}

func TestClassifyCommandDoesNotForbidAnArgumentThatNamesADestructiveCommand(t *testing.T) {
	for command, want := range map[string]CommandClass{
		"last reboot | head -n 5":          CommandReadOnly,
		"grep -c shutdown /var/log/syslog": CommandReadOnly,
		"rm -rf /srv/app/tmp":              CommandChange,
		"dd if=/dev/zero of=/dev/null":     CommandChange,
		"grep reboot \"$LOG\"":             CommandChange,
	} {
		if got := ClassifyCommand(command); got != want {
			t.Errorf("ClassifyCommand(%q) = %s, want %s", command, got, want)
		}
	}
}

func TestIsReadOnlyInvocation(t *testing.T) {
	for _, action := range []string{ActionSystemSnapshot, ActionServiceStatus, ActionJournalTail} {
		if !IsReadOnlyInvocation(action, nil) {
			t.Errorf("%s is not read-only", action)
		}
	}
	for _, action := range []string{ActionServiceRestart, ActionServiceStart, ActionServiceStop, ActionServiceReload, "unknown"} {
		if IsReadOnlyInvocation(action, map[string]any{"unit": "nginx.service"}) {
			t.Errorf("%s is read-only", action)
		}
	}
	if !IsReadOnlyInvocation(ActionCommandRun, map[string]any{"command": "df -h"}) {
		t.Error("a read-only command is not read-only")
	}
	for _, input := range []map[string]any{
		{"command": "systemctl restart nginx"},
		{"command": "rm -rf /"},
		{"command": "df -h", "extra": true},
		{},
	} {
		if IsReadOnlyInvocation(ActionCommandRun, input) {
			t.Errorf("IsReadOnlyInvocation(%v) = true", input)
		}
	}
}

func TestCommandFromInputRefusesAForbiddenCommand(t *testing.T) {
	_, err := commandFromInput(map[string]any{"command": "rm -rf /"})
	if !errors.Is(err, errReadActionInput) {
		t.Fatalf("commandFromInput() error = %v, want invalid input", err)
	}
	if _, err := commandFromInput(map[string]any{"command": "rm -rf /srv/app/tmp"}); err != nil {
		t.Fatalf("a change was refused: %v", err)
	}
}
