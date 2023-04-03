import {ethers} from 'ethers';
import {assert} from 'chai';
import 'mocha';
import * as fs from 'fs';


describe("CIP20", function () {
  let cip20: any;
  this.timeout(25000);

  before(async () => {
    const provider = new ethers.providers.JsonRpcProvider(process.env.npm_config_networkaddr);
    const signerKey: any = ethers.utils.hexlify(process.env.npm_config_signerkey!)
    const signer = new ethers.Wallet(signerKey, provider);

    const contractJSON = fs.readFileSync("../../compiled-system-contracts/CIP20Test.json", 'utf8');
    const CIP20TestFactory = ethers.ContractFactory.fromSolidity(contractJSON, signer);
    cip20 = await CIP20TestFactory.deploy();
  });

  it("should run hashes", async () => {
    const vectors = require("./cip20.json")
    for (const vector of vectors) {
      let preimage = `0x${vector.preimage}`;

      assert.include(await cip20.sha2_512(preimage), `0x${vector.sha2_512}`);
      assert.include(await cip20.keccak512(preimage), `0x${vector.keccak512}`);
      assert.include(await cip20.sha3_256(preimage), `0x${vector.sha3_256}`);
      assert.include(await cip20.sha3_512(preimage), `0x${vector.sha3_512}`);
      assert.include(await cip20.blake2s(preimage), `0x${vector.blake2s}`);
    }
  });
});
