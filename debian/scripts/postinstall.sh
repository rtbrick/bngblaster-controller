#!/bin/bash
cat <<EOF
rtbrick-bngblasterctrl has been installed as a systemd service
EOF
systemctl daemon-reload;
systemctl start rtbrick-bngblasterctrl;
systemctl enable rtbrick-bngblasterctrl;