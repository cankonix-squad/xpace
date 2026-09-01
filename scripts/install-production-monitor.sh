#!/bin/sh
set -eu

project_dir=${XPACE_PROJECT_DIR:-/root/cankonix-node/apps/xpace}
install -m 0644 "$project_dir/infra/systemd/xspace-monitor.service" /etc/systemd/system/xspace-monitor.service
install -m 0644 "$project_dir/infra/systemd/xspace-monitor.timer" /etc/systemd/system/xspace-monitor.timer
systemctl daemon-reload
systemctl enable --now xspace-monitor.timer
systemctl start xspace-monitor.service
systemctl --no-pager status xspace-monitor.timer
