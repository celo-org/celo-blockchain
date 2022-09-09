import {ethers} from "ethers";

// argv has the arguments index zero is node index 1 is this file and index2 is the network address
const provider = new ethers.providers.JsonRpcProvider(process.argv[2]);


async function gb() {
	let b = await provider.getBlock("latest")
	console.log(b)
}

gb();
