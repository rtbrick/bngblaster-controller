#!/bin/bash
# Debian passes "configure" plus the previously installed version on upgrade,
# and an empty second argument on a fresh install.
systemctl daemon-reload

if [ -z "$2" ]; then
    systemctl enable --now rtbrick-bngblasterctrl
    cat <<MSG
rtbrick-bngblasterctrl has been installed as a systemd service
Edit /etc/default/rtbrick-bngblasterctrl to configure its command-line options

With the default options the web UI is served on http://<host>:8001/
WARNING: the web UI and REST API do not require authentication yet;
restrict access to the port or disable features as described in
/etc/default/rtbrick-bngblasterctrl
MSG
else
    # Upgrade: pick up the new binary without touching the enabled/disabled
    # state the administrator chose, and only if the service was running.
    systemctl try-restart rtbrick-bngblasterctrl
fi
exit 0
