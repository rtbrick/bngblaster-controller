#!/bin/bash
# Debian also runs prerm with "upgrade"; the service is restarted by
# postinst in that case, so only stop and disable it on an actual removal.
if [ "$1" = "remove" ]; then
    systemctl stop rtbrick-bngblasterctrl
    systemctl disable rtbrick-bngblasterctrl
fi
exit 0
