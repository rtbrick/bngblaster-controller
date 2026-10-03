#!/bin/bash
# The unit file is gone by now, so let systemd forget about it.
if [ "$1" = "remove" ] || [ "$1" = "purge" ]; then
    systemctl daemon-reload
    systemctl reset-failed rtbrick-bngblasterctrl 2>/dev/null
fi
exit 0
