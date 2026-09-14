#!/bin/bash
cat <<EOF
rtbrick-bngblasterctrl has been installed as a systemd service
Edit /etc/default/rtbrick-bngblasterctrl to configure its command-line options
EOF
systemctl daemon-reload;
systemctl start rtbrick-bngblasterctrl;
systemctl enable rtbrick-bngblasterctrl;