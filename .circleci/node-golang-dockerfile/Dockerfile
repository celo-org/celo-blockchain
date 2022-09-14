# This has been pushed to us.gcr.io/celo-testnet/circleci-node12-golang1.17.5
FROM circleci/node:12

COPY --from=circleci/golang:1.17.5 /usr/local/go/ /usr/local/go/
 
ENV PATH="/usr/local/go/bin:${PATH}"
