/*
Package downloader handles downloading data from other nodes for sync. Sync
referrs to the process of catching up with other nodes, once caught up nodes
use a different process to maintain their syncronisation.

There are a few different modes for syncing

Full: Get all the blocks and apply them all to build the chain state

Fast: Get all the blocks and block receipts and insert them in the db without
processing them and at the same time download the state for a block near the
head block. Once the state has been downloaded, process subsequent blocks as a
full sync would in order to reach the tip of the chain.

Fast sync introduces the concept of the pivot, which is a block at some point
behind the head block for which the node attempts to sync state for. In geth
the pivot was chosen to be 64 blocks behind the head block, the reason for
choosing a point behind the head was to ensure that the block that you are
syncing state for a block which is on the main chain and won't get reorged out.
(see https://github.com/ethereum/go-ethereum/issues/25100), it was called the
pivot because before the pivot the fast sync approach is used but after the
pivot full sync is used, so you could imagine the syncing strategy pivoting
around that point.

In celo we don't have the problem of reorgs but we still retain the pivot point
because the validator uptime scores historically were required to be calculated
by processing blocks from an epoch boundary. However since
https://github.com/celo-org/celo-blockchain/pull/1833 which removes the
requirement to process blocks from an epoch boundary we could in fact drop the
concept of pivot.

Snap: Not currently working, but in theory works like fast sync except that
nodes download a flat file to get the state, as opposed to making hundreds of
thousands of individual requests for it. This should significantly speed up
sync.

Light: Downloads only headers during sync and then downloads other data on
demand in order to service rpc requests.

Lightest: Like light but downloads only one header per epoch, which on mainnet
means one header out of every 17280 headers. This is particularly fast only
takes 20 seconds or so to get synced.

Sync process detail

Syncing is initiated with one peer (see eth.loop), the peer selected to sync
with is the one with the highest total difficulty of all peers (see
eth.nextSyncOp). Syncing may be cancelled and started with a different peer if
a peer with a higher total difficulty becomes available.

Syncing introduces the concept of a checkpoint (see params.TrustedCheckpoint).
The checkpoint is a hard coded set of trie roots that allow state sync to start
before the whole header chain has been downloaded.

The pivot point is the point which the fast sync syncs state for, it is
calculated as the first block of the epoch containing the block that is
fsMinFullBlocks behind the current head block of the peer we are syncing
against (fsMinFullBlocks is hardcoded to 64, chosen by the geth team to make
re-orgs of the synced state unlikely).

The first step in syncing with a peer is to fetch the latest block header and
pivot header. The geth implementation simply calculates the pivot as being 64
blocks before (fsMinFullBlocks) the head, and so if the head is currently < 64
then there is no valid pivot, in that case geth code uses the head as the pivot
(they say to avoid nil pointer exceptions, but its not clear what this will do
to the sync). From the celo side there should never be a case without a pivot
block because we instead choose the pivot to be zero if head is currently < 64.

Next the sync finds the common ancestor (aka origin) between the node and the
peer is syncing against.

If fast syncing {

	The pivot is written to a file.

	If the origin turns out to be after the pivot then it is set to be just
	before the pivot.

	The ancient limit is set on the downloader (it would be much nicer if the
	concept of ancient could be encapsulated in the database rather than
	leaking here). The ancient defines boundary between freezer blocks and
	current blocks. Setting ancient limit here enables "direct-ancient mode"
	which I guess bypasses putting stuff into the main chain and then having it
	be moved to the freezer later. I guess in full sync mode since all blocks
	need to be processed all blocks need to go into the main database first and
	only after they have been process can they be moved to the freezer, but
	since fast sync does not process all blocks that step can be skipped.

	Then, and I'm not really clear why if the origin is greater than the last
	frozen block (IE there is stuff in the current database beyond whats in the
	Freezer) the "direct-ancient mode is disabled", maybe because it is only
	applicable for nodes that are starting from scratch or have never reached
	the pivot.

	If the origin turns out to be lower than the most recent frozen block then
	the blockchain is rewound to the origin.

	set the pivotHeader on the downloader as the pivot.

}

Then a number of go routines are started to fetch data from the origin to the head.

fetchHeaders
fetchBodies
fetchReceipts

And one to process the headers. (processHeaders)

If fast sync {
	start a routine to process the fast sync content (processFastSyncContent)
}
If full syncing {

	start a routine to process the full sync content (processFullSyncContent)
}


These goroutines form a pipeline where the downloaded data flows as follows.

							   -> fetchBodies -> processFullSyncContent
                              /               \
fetchHeaders -> processHeaders                 \
                              \                 \
							   -> fetchReceipts --> processFastSyncContent



fetchHeaders

fetchHeaders introduces the skeleton concept. The idea is that the node
requests a set of headers from the peer that are spaced out at regular
intervals, and then uses all peers to request headers to fill the gaps. The
header hashes allow the node to easily verify that the received headers match
the chain of the main peer they are syncing against. Whether fetching skeleton
headers or not requests for headers are done in batches of up to 192
(MaxHeaderFetch) headers.

If lightest sync {
	fetch just epoch headers till current epoch then fetch all subsequent headers. (no skeleton)
} else {
	fetch  headers using the skeleton approach, until no more skeleton headers
	are returned then switch to requesting all subsequent headers from the
	peer.
}


Wait for headers to be received.

Pass the received headers to the processHeaders routine.

If no more headers are returned and the pivot state has been fully synced then
exit. (The pivot being synced is communicated via an atomic from
processFastSyncContent)

Fetch more headers as done to start with.

processHeaders

Waits to receive headers from fetchHeaders inserts the received headers into the header chain.

If full sync {
	request blocks for inserted headers. (fetchBodies)
}
If fast sync {
	request blocks and receipts for inserted headers. (fetchBodies & fetchReceipts)
}

processFastSyncContent

Reads fetch results (each fetch result has all the data required for a block
(header txs, receipts, randomnes & epochSnarkData)from the downloader queue.

Updates the pivot block point if it has fallen sufficiently behind head.

Splits the fetch results around the pivot.

Results before the pivot are inserted with BlockChain.InsertReceiptChain (which
inserts receipts, because in fast sync most blocks are not processed) and those
after the pivot

If the pivot has completed syncing {
	Inserts the results after the pivot with, BlockChain.InsertChain and exits.
} else {
	Start the process again prepending the results after the pivot point to the
	newly fetched results. (Note that if the pivot point is subsequently
	updated those results will be processed as fast sync results and inserted
	via BlockChain.InsertReceiptChain, but there seems to be a problem with our
	current implementation that means that the pivot would have to get 2 days
	old before it would be updated, so actually it looks like the list of
	result s will grow a lot during this time could be an OOM consideration)
}


fetchBodies

A routine that gets notified of bodies to fetch and calls into a beast of a
function (fetchParts) to fetch batches of block bodies from different peers,
those bodies are delivered to the queue that collates them along with other
delivered data into fetch results that are then retrieved by either
processFastSyncContent or processFullSyncContent.

fetchReceipts

like fetchBodies but for receipts.
*/
package downloader
