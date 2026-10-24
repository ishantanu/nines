#!/bin/bash
set -euo pipefail
umask 077
trap 'printf "bootstrap failed at line %s\n" "$LINENO"' ERR
mkdir -p /etc/ceph /var/lib/ceph/mon/ceph-a /var/lib/ceph/osd/ceph-0 /var/log/ceph /run/ceph
fsid=$(cat /proc/sys/kernel/random/uuid)
cat > /etc/ceph/ceph.conf <<EOF
[global]
fsid = $fsid
mon initial members = a
mon host = 127.0.0.1
public network = 127.0.0.0/8
cluster network = 127.0.0.0/8
setuser = root
setgroup = root
osd pool default size = 1
osd pool default min size = 1
osd pool default pg num = 8
osd pool default pgp num = 8
osd pool default pg autoscale mode = off
mon allow pool size one = true
mon warn on pool no redundancy = false
ms bind ipv6 = false
[mon]
mon data = /var/lib/ceph/mon/ceph-a
[osd.0]
osd data = /var/lib/ceph/osd/ceph-0
osd objectstore = bluestore
bluestore block size = 2147483648
bluestore block create = true
bluestore block db size = 67108864
bluestore cache autotune = false
bluestore cache size = 67108864
osd memory target = 268435456
osd op num shards = 1
osd op num threads per shard = 1
[client.rgw.fixture]
rgw frontends = beast ssl_port=9443 ssl_certificate=/fixture/server.pem
rgw thread pool size = 4
rgw crypt sse s3 backend = vault
rgw crypt sse s3 vault auth = token
rgw crypt sse s3 vault addr = http://127.0.0.1:8200
rgw crypt sse s3 vault prefix = /v1/transit
rgw crypt sse s3 vault token file = /fixture/bao-token
EOF
ceph-authtool --create-keyring /tmp/mon.keyring --gen-key -n mon. --cap mon 'allow *' >/dev/null
ceph-authtool --create-keyring /etc/ceph/ceph.client.admin.keyring --gen-key -n client.admin --cap mon 'allow *' --cap osd 'allow *' --cap mgr 'allow *' >/dev/null
ceph-authtool /tmp/mon.keyring --import-keyring /etc/ceph/ceph.client.admin.keyring >/dev/null
monmaptool --create --add a 127.0.0.1 --fsid "$fsid" /tmp/monmap >/dev/null
ceph-mon --mkfs -i a --monmap /tmp/monmap --keyring /tmp/mon.keyring > /var/log/ceph/mon-mkfs.log 2>&1
ceph-mon -i a -f >/var/log/ceph/mon-console.log 2>&1 &
ready=false
for attempt in $(seq 1 60); do
  if timeout 2 ceph status >/dev/null 2>&1; then ready=true; break; fi
  sleep 1
done
$ready || { echo "Monitor startup timed out"; exit 1; }
ceph mon enable-msgr2
ceph config set mon auth_allow_insecure_global_id_reclaim false
mkdir -p /var/lib/ceph/mgr/ceph-a
ceph auth get-or-create mgr.a mon 'allow profile mgr' osd 'allow *' mds 'allow *' >/var/lib/ceph/mgr/ceph-a/keyring
ceph-mgr -i a -f >/var/log/ceph/mgr-console.log 2>&1 &
ceph osd create >/dev/null
ceph auth get-or-create osd.0 mon 'profile osd' mgr 'profile osd' osd 'allow *' >/var/lib/ceph/osd/ceph-0/keyring
ceph-osd -i 0 --mkfs --no-mon-config --osd-uuid "$(cat /proc/sys/kernel/random/uuid)" >/var/log/ceph/osd-mkfs.log 2>&1
ceph osd crush add osd.0 1.0 root=default host=ceph-fixture >/dev/null
ceph-osd -i 0 -f >/var/log/ceph/osd-console.log 2>&1 &
ready=false
for attempt in $(seq 1 60); do
  if ceph osd stat -f json | python3 -c 'import json,sys; sys.exit(json.load(sys.stdin)["num_up_osds"] != 1)'; then ready=true; break; fi
  sleep 1
done
$ready || { echo "OSD startup timed out"; exit 1; }
ceph auth get-or-create client.rgw.fixture mon 'allow rw' osd 'allow rwx' >/etc/ceph/ceph.client.rgw.fixture.keyring
radosgw-admin user create --uid=nines-acceptance --display-name='Nines synthetic acceptance' --access-key="$(cat /fixture/access-key)" --secret-key="$(cat /fixture/secret-key)" >/dev/null
printf '%s\n' 'Cluster ready; starting TLS RGW'
exec radosgw -n client.rgw.fixture -f --keyring /etc/ceph/ceph.client.rgw.fixture.keyring
