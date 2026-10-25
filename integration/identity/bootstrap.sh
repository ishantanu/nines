#!/bin/sh
set -eu
umask 077
mkdir -p /run/identity /data/ldap
admin_hash=$(slappasswd -T /fixture/admin-password)
reader_hash=$(slappasswd -T /fixture/reader-password)
user_hash=$(slappasswd -T /fixture/user-password)
cat > /run/identity/slapd.conf <<CONFIG
include /etc/ldap/schema/core.schema
include /etc/ldap/schema/cosine.schema
include /etc/ldap/schema/inetorgperson.schema
pidfile /run/identity/slapd.pid
argsfile /run/identity/slapd.args
modulepath /usr/lib/ldap
moduleload back_mdb
TLSCACertificateFile /fixture/ca.pem
TLSCertificateFile /fixture/ca.pem
TLSCertificateKeyFile /fixture/server-key.pem
security simple_bind=128
database mdb
maxsize 67108864
suffix "dc=example,dc=test"
rootdn "cn=admin,dc=example,dc=test"
rootpw $admin_hash
directory /data/ldap
access to attrs=userPassword by self write by anonymous auth by * none
access to * by dn.exact="cn=reader,dc=example,dc=test" read by users read by * none
CONFIG
cat > /run/identity/seed.ldif <<LDIF
dn: dc=example,dc=test
objectClass: dcObject
objectClass: organization
o: Disposable Nines identity acceptance
dc: example

dn: ou=people,dc=example,dc=test
objectClass: organizationalUnit
ou: people

dn: cn=reader,dc=example,dc=test
objectClass: organizationalRole
objectClass: simpleSecurityObject
cn: reader
userPassword: $reader_hash

dn: uid=alice,ou=people,dc=example,dc=test
objectClass: inetOrgPerson
uid: alice
cn: Alice Fixture
sn: Fixture
displayName: Alice Fixture
mail: alice@example.test
userPassword: $user_hash

dn: uid=bob,ou=people,dc=example,dc=test
objectClass: inetOrgPerson
uid: bob
cn: Bob Fixture
sn: Fixture
displayName: Bob Fixture
mail: bob@example.test
userPassword: $user_hash
LDIF
slapadd -f /run/identity/slapd.conf -l /run/identity/seed.ldif
exec slapd -f /run/identity/slapd.conf -h 'ldap://0.0.0.0:1389/ ldaps://0.0.0.0:1636/' -d 0
