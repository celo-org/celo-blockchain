import {ethers} from "ethers";
import {assert} from 'chai';
import 'mocha';

describe('ethers.js compatibility', () => {

	it('block retrieved with gasLimit', async () => {
		let provider = new ethers.providers.JsonRpcProvider(process.env.npm_config_networkaddr);
		let block = await provider.getBlock(process.env.npm_config_blocknum as string);

		// These assertions trigger on undefined or null
		assert.notEqual(block, null);
		assert.notEqual(block.gasLimit, null);
	});

	it('EIP-1559 transactions supported', async () => {
		let provider = new ethers.providers.JsonRpcProvider(process.env.npm_config_networkaddr);

		// The fee data is the construct used to determine if EIP-1559 transactions are supported, if it contains max
		let feeData = await provider.getFeeData();

		// These assertions trigger on undefined or null
		assert.notEqual(feeData, null);
		// If the following 2 fields are set then the network is assumed to support EIP-1559 transactions.
		assert.notEqual(feeData.maxFeePerGas, null);
		assert.notEqual(feeData.maxPriorityFeePerGas, null);
		// We check the other 2 fields for completeness, they should also be set.
		assert.notEqual(feeData.gasPrice, null);
		assert.notEqual(feeData.lastBaseFeePerGas, null);
	});

	// Our blockchain client implementation returns a default gas limit when the
	// actual gas limit cannot be retrieved.  We cannot check fee data against a
	// pruned block because getFeeData always requests the latest block.
	it('block with pruned state still reports gas limit', async () => {
		let provider = new ethers.providers.JsonRpcProvider(process.env.npm_config_networkaddr);
		let block = await provider.getBlock(process.env.npm_config_blocknum as string);

		// These assertions trigger on undefined or null
		assert.notEqual(block, null);
		assert.notEqual(block.gasLimit, null);
	});

});
