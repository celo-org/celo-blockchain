#!/bin/sh

case "$1" in
  validator)
    ./build/bin/geth --datadir ~/mydata/datadir --syncmode full --gcmode archive --debug --port 30303 --rpcvhosts='*' --networkid 1101 --verbosity=3 --consoleoutput=stdout --consoleformat=term --nat extip:127.0.0.1 --allow-insecure-unlock --rpc --rpcport 8545 --rpccorsdomain='*' --rpcapi=eth,net,web3,debug,admin,personal,txpool,istanbul --light.serve=0 --mine --minerthreads=10 --nodekeyhex=add67e37fdf5c26743d295b1af6d9b50f2785a6b60bc83a8f05bd1dd4b385c6c --istanbul.blockperiod 10 --nodiscover --password=/dev/null --unlock=0  --ws --wsaddr 0.0.0.0 --rpcaddr 0.0.0.0 --wsapi=eth,net,web3,debug,personal --wsorigins='*' --wsport 8546
    ;;
  fast)
    ./build/bin/geth --datadir ~/fasttest/validator --syncmode full --gcmode archive --debug --port 30303 --rpcvhosts='*' --networkid 1101 --verbosity=3 --consoleoutput=stdout --consoleformat=term --nat extip:127.0.0.1 --allow-insecure-unlock --rpc --rpcport 8545 --rpccorsdomain='*' --rpcapi=eth,net,web3,debug,admin,personal,txpool,istanbul --light.serve=0 --mine --minerthreads=10 --nodekeyhex=add67e37fdf5c26743d295b1af6d9b50f2785a6b60bc83a8f05bd1dd4b385c6c --istanbul.blockperiod 1 --nodiscover --password=/dev/null --unlock=0  --ws --wsaddr 0.0.0.0 --rpcaddr 0.0.0.0 --wsapi=eth,net,web3,debug,personal --wsorigins='*' --wsport 8546
    ;;
  node)
    ./build/bin/geth --datadir ~/mydata/nextnode --syncmode full --debug --port 30301 --rpcvhosts='*' --networkid 1101 --verbosity=3 --nat extip:127.0.0.1 --allow-insecure-unlock --rpc --rpcport 9545 --istanbul.blockperiod 1 --rpccorsdomain='*' --rpcapi=eth,net,web3,debug,admin,personal,txpool,istanbul --light.serve=0 --nodiscover --unlock=0xfD5d2142281fbfA75439f94774506fE76641CDd6 --password=asd
    ;;
  spammer)
    celo --datadir ~/mydata/spammer --syncmode full --debug --port 30302 --rpcvhosts='*' --networkid 1101 --verbosity=3 --nat extip:127.0.0.1 --allow-insecure-unlock --rpc --rpcport 10545 --rpccorsdomain='*' --rpcapi=eth,net,web3,debug,admin,personal,txpool,istanbul --light.serve=0 --nodiscover --unlock=0xAdB5B5DfB87dd7652796c3b91Ff27001d55CCca6 --password=asd
    ;;
  faulty)
    celo --datadir ~/mydata/faulty --syncmode full --debug --port 30302 --rpcvhosts='*' --networkid 1101 --verbosity=3 --nat extip:127.0.0.1 --allow-insecure-unlock --rpc --rpcport 10545 --rpccorsdomain='*' --rpcapi=eth,net,web3,debug,admin,personal,txpool,istanbul --light.serve=0 --nodiscover
    ;;
  connect)
    ./build/bin/geth --datadir ~/mydata/datadir attach --exec 'admin.addPeer("enode://619702d99a2c3161ab816e0e1ca82abba8ab95edbb7702903f9052204b2c8194affbf6564d822643f7a24ef97eb42982c966b4b1857a8f3e2b3a9ef32b519570@127.0.0.1:30301")'
    ./build/bin/geth --datadir ~/mydata/datadir attach --exec 'admin.addPeer("enode://a1403c44bad29b0efd734f4629484c7da8d52f28dbbc4bc2d96a222aa883549be5abc6f0adbf79259cbe82ec9b8452c2b2b35b960a2f99ef50b33909f48b1f0d@127.0.0.1:30302")'
    ;;
esac

# ./build/bin/geth --datadir ~/mydata/datadir --syncmode full --gcmode archive --debug --port 30303 --rpcvhosts='*' --networkid 1101 --verbosity=3 --consoleoutput=stdout --consoleformat=term --nat extip:127.0.0.1 --allow-insecure-unlock --rpc --rpcport 8545 --rpccorsdomain='*' --rpcapi=eth,net,web3,debug,admin,personal,txpool,istanbul --light.serve=0 --mine --minerthreads=10 --nodekeyhex=add67e37fdf5c26743d295b1af6d9b50f2785a6b60bc83a8f05bd1dd4b385c6c --istanbul.blockperiod 10 --nodiscover --password=/dev/null --unlock=0  --ws --wsaddr 0.0.0.0 --rpcaddr 0.0.0.0 --wsapi=eth,net,web3,debug,personal --wsorigins='*' --wsport 8546
