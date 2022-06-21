/*
Package rawdb contains a collection of low level database accessors.

The rawdb serves as a wrapper around level db and a file based database
providing an ethereum specific interface.

The rawdb introduces the concept of ancient items, which are stored in the
freezer (a flat file based db) rather than in leveldb, this structure was
chosen to improve performance and reduce memory consumption. See this
PR (https://github.com/ethereum/go-ethereum/pull/19244) for more detail.

The freezer is split into a number of tables, each holding a different type of data.

They are:

* block headers
* canonical block hashes
* block bodies
* receipts
* total difficulty

Each table contains one entry per block and the data for each table is stored
in a file on disk that is only ever appended to. If a file grows beyond a
certain threshold then subsequent entries for that table are appended into a
new sequentially numbered file.

Sometimes the tables can get out of sync and in such situations they are all
truncated to match the table with the smallest amount of entries.

The freezer starts a background routine to periodically collect values written
to leveldb and move them to freezer files. Since data in the freezer cannot be
modified the items added to the freezer must be considered final (I.E, no
chance of a re-org) currently that is ensured by only freezing blocks at least
90000 behind the head block. This 90000 threshold is referred to as the
'FullImmutabilityThreshold'.

*/
package rawdb
