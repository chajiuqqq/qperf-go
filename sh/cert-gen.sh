# pk
#openssl genpkey -algorithm RSA -out private.key
openssl req -new -key ./cert/private.key -out ./cert/csr.csr -config server.req
openssl x509 -req -in ./cert/csr.csr -signkey ./cert/private.key -out ./cert/certificate.crt -extensions v3_req -extfile server.req