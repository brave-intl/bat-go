# Create Vault

First step in process is to create operator x25519 identities.  This can be accomplished
by using the `age-keygen` tool or `ssh-keygen` tool.  Shown below is how you can create
5 key pairs we can use with create vault command.

```bash
# create operator encryption keys
for i in `seq 1 5`; do age-keygen -o key${i}.txt; done
Public key: age1l7envedtgx7tpkqt8lt9hpc7sv87dhxmmlgnewxhwknrg0vcccgsf35etj
Public key: age1q6tpe67stm5pexes6yk40r6400fhzudjx4mj730zpxw6ftfvnefqc5qyn7
Public key: age1nf60d46496795s8j9zmt5y2jwfhz077r2nx7sverta2rr8ck3pqsh4dmv3
Public key: age1ndh9gjz83tmfevhc759wyuvh6656xwgv34uun8jegxw077rs3ceswpm4vl
Public key: age1gz7lx6ntlszz7dlp3hcjhg32fh99cwcsv92dkeqr4wejc8jc3srs9pmrz8
```

Next we can run the create vault command and specify each of the public keys as arguments to the command:

```bash
go run main.go age1l7envedtgx7tpkqt8lt9hpc7sv87dhxmmlgnewxhwknrg0vcccgsf35etj \
    age1q6tpe67stm5pexes6yk40r6400fhzudjx4mj730zpxw6ftfvnefqc5qyn7 \
    age1nf60d46496795s8j9zmt5y2jwfhz077r2nx7sverta2rr8ck3pqsh4dmv3 \
    age1ndh9gjz83tmfevhc759wyuvh6656xwgv34uun8jegxw077rs3ceswpm4vl \
    age1gz7lx6ntlszz7dlp3hcjhg32fh99cwcsv92dkeqr4wejc8jc3srs9pmrz8
```

This command will then create Shamir shares for each of these identities, and encrypt said share with
the identity, and output a share ciphertext file.

```bash
share-age1gz7lx6ntlszz7dlp3hcjhg32fh99cwcsv92dkeqr4wejc8jc3srs9pmrz8
share-age1l7envedtgx7tpkqt8lt9hpc7sv87dhxmmlgnewxhwknrg0vcccgsf35etj
share-age1ndh9gjz83tmfevhc759wyuvh6656xwgv34uun8jegxw077rs3ceswpm4vl
share-age1nf60d46496795s8j9zmt5y2jwfhz077r2nx7sverta2rr8ck3pqsh4dmv3
share-age1q6tpe67stm5pexes6yk40r6400fhzudjx4mj730zpxw6ftfvnefqc5qyn7
```

You can verify that you can decrypt the share file with your private key by performing the
check below: 

```bash
age --decrypt -i key1.txt share-age1l7envedtgx7tpkqt8lt9hpc7sv87dhxmmlgnewxhwknrg0vcccgsf35etj
K5dF/YbKN84ScNFkXq4/X17RWgFVkqWupeSfjGoMX2qnaJ6c74JucEupXwmR/w8O/z9hE+psIe246r/9tnLCxrNTsXhY8Uu4zZSy
```

The output "share-" files will be used as input to the "bootstrap" command.
