package workers

import "text/template"

var (
	initScript = template.Must(template.New("init-script").Parse(`#!/bin/bash
set -o errexit

cd /tmp

cat > docker_rsa <<EOF
{{.DockerRSA}}
EOF

cat > travis-worker.yml <<EOF
{{.TravisWorkerYML}}
EOF

cat > papertrail.conf <<EOF
\$DefaultNetstreamDriverCAFile /etc/papertrail.crt
\$DefaultNetstreamDriver gtls
\$ActionSendStreamDriverMode 1
\$ActionSendStreamDriverAuthMode x509/name

*.* @@{{.PapertrailSite}}
EOF

cat > watch-files.conf <<EOF
\$ModLoad imfile
\$InputFileName /etc/sv/travis-worker/log/main/current
\$InputFileTag travis-worker
\$InputFileStateFile state_file_worker_log
\$InputFileFacility local7
\$InputRunFileMonitor
\$InputFilePollInterval 10
EOF

mkdir /home/deploy/.ssh
chown travis:travis /home/deploy/.ssh
chmod 0700 /home/deploy/.ssh
mv docker_rsa /home/deploy/.ssh/docker_rsa
chown travis:travis /home/deploy/.ssh/docker_rsa
chmod 0600 /home/deploy/.ssh/docker_rsa
mv travis-worker.yml /home/deploy/travis-worker/config/worker.yml
chown travis:travis /home/deploy/travis-worker/config/worker.yml
chmod 0600 /home/deploy/travis-worker/config/worker.yml
mv watch-files.conf /etc/rsyslog.d/60-watch-files.conf
mv papertrail.conf /etc/rsyslog.d/65-papertrail.conf
service rsyslog restart

INSTANCE_ID=$(curl -s http://169.254.169.254/latest/meta-data/instance-id)
echo > metadata
for attr in hostname instance-type public-hostname public-ipv4 ; do
  echo -en "$attr=$(curl -s http://169.254.169.254/latest/meta-data/$attr)&" >> metadata
end
echo -en "instance-id=$INSTANCE_ID" >> metadata

curl -f -d @metadata -X PATCH {{.InstanceMetadataURL}}
curl -f -d 'state=finished' -X PATCH {{.InstanceBuildURL}}
`))
)

type initScriptContext struct {
	InstanceMetadataURL string
	DockerRSA           string
	PapertrailSite      string
	TravisWorkerYML     string
	InstanceBuildID     string
	InstanceBuildURL    string
}