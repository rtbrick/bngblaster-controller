#!/bin/bash
systemctl stop rtbrick-bngblasterctrl;
systemctl disable rtbrick-bngblasterctrl;
rm /etc/systemd/system/rtbrick-bngblasterctrl.service;
systemctl daemon-reload;
systemctl reset-failed;