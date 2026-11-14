#!/data/data/com.termux/files/usr/bin/bash
# Wrapper to run ngrok inside termux-chroot with valid SSL certificates
exec /data/data/com.termux/files/usr/bin/termux-chroot env SSL_CERT_FILE=/etc/tls/cert.pem ngrok "$@" --log=/home/go-app/ngrok.log --log-level=info
