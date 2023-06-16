/*
 * Note these tests are intended to be invoked only by our e2e tests, they
 * should not be executed in a standalone fashion.
 * See e2e_test.TestEthersJSCompatibility
 */
import {ethers} from 'ethers';
import {assert} from 'chai';
import 'mocha';

describe('ethers.js compatibility tests with state', () => {

	it('provider.getBlock works (block has gasLimit set)', async () => {
		let provider = new ethers.JsonRpcProvider(process.env.npm_config_networkaddr);
		let block = await provider.getBlock(process.env.npm_config_blocknum as string);

		// These assertions trigger on undefined or null
		assert.notEqual(block, null);
		assert.notEqual(block!.gasLimit, null);
	});

	it('EIP-1559 transactions supported (can get feeData)', async () => {
		let provider = new ethers.JsonRpcProvider(process.env.npm_config_networkaddr);

		// The fee data is the construct used to determine if EIP-1559 transactions are supported, if it contains max
		let feeData = await provider.getFeeData();

		// These assertions trigger on undefined or null
		assert.notEqual(feeData, null);
		// If the following 2 fields are set then the network is assumed to support EIP-1559 transactions.
		assert.notEqual(feeData.maxFeePerGas, null);
		assert.notEqual(feeData.maxPriorityFeePerGas, null);
		// We check the other 2 fields for completeness, they should also be set.
		assert.notEqual(feeData.gasPrice, null);
		// assert.notEqual(feeData.lastBaseFeePerGas, null);
	});

	it('block has gasLimit', async () => {
		let provider = new ethers.JsonRpcProvider(process.env.npm_config_networkaddr);
		const fullBlock = await provider.send(
			'eth_getBlockByNumber',
			[ethers.toQuantity(process.env.npm_config_blocknum as string), true]
		)
		assert.isTrue(fullBlock.hasOwnProperty('gasLimit'))
	});

	it('block has baseFeePerGas', async () => {
		let provider = new ethers.JsonRpcProvider(process.env.npm_config_networkaddr);
		const fullBlock = await provider.send(
			'eth_getBlockByNumber',
			[ethers.toQuantity(process.env.npm_config_blocknum as string), true]
		)
		assert.isTrue(fullBlock.hasOwnProperty('baseFeePerGas'))
	});

});

describe('ethers.js compatibility tests with no state', () => {

	it('block has gasLimit', async () => {
		let provider = new ethers.JsonRpcProvider(process.env.npm_config_networkaddr);
		const fullBlock = await provider.send(
			'eth_getBlockByNumber',
			[ethers.toQuantity(process.env.npm_config_blocknum as string), true]
		)
		assert.isTrue(fullBlock.hasOwnProperty('gasLimit'))
	});

	it('block has no baseFeePerGas', async () => {
		let provider = new ethers.JsonRpcProvider(process.env.npm_config_networkaddr);
		const fullBlock = await provider.send(
			'eth_getBlockByNumber',
			[ethers.toQuantity(process.env.npm_config_blocknum as string), true]
		)
		assert.isFalse(fullBlock.hasOwnProperty('baseFeePerGas'))
	});


});
