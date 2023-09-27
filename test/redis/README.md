The Redis tests use TLS. 

To generate certificates and keys use the script in the `gen-test-cert.sh` (in this directory) which will generate the 
necessary files and put them in the ./test/redis/tls directory. These will ne included in the docker 
build and used the master and slave nodes.

Redis also uses acl for permissions, these can be found in the users.acl file.