#!/bin/bash

# We consider a good sync if the latest block synced is OLDEST_ACCEPTABLE seconds old
OLDEST_ACCEPTABLE=30

if [ -z "$DATADIR" ]; then
  echo "Set DATADIR to the desired datadir folder"
  exit 3
fi

if [ -z "$MODE" ]; then
  echo "Set MODE to the sync mode"
  exit 3
fi
LOGFILE=/tmp/sync_test.log

# Do the sync
echo "Running geth sync"
build/bin/geth --datadir $DATADIR --syncmode $MODE --exitwhensynced > $LOGFILE 2>&1

MARK=`date +%s`

# Now check what the latest block is
ATTEMPTS=10
RETRY_SLEEP=3
# We attempt to check it several times since, sometimes the command
# fails with "No peers available"
for ATTEMPT in $(seq 1 $ATTEMPTS); do
	echo "Attempt $ATTEMPT/$ATTEMPTS of getting the latest block timestamp" 
	LATEST=`build/bin/geth --datadir $DATADIR --verbosity 0 console --syncmode $MODE --exec 'eth.getBlock("latest").timestamp'`
	RESULT=$?
	# If the execution returned 0, and the output is a number...
	if [ $RESULT -eq 0 ] && [ $LATEST -eq $LATEST 2> /dev/null ]; then
		DIFF="$(($LATEST - $MARK))"
		SUM="$((DIFF + OLDEST_ACCEPTABLE))"
		if [ "$SUM" -gt 0 ]; then
			echo "Sync successful"
			exit 0
		else
			echo "Sync failed. Latest block is $DIFF seconds old"
			exit 1
		fi
	else
		# retry
		echo "Attempt $ATTEMPT failed, got: $LATEST"
		sleep $RETRY_SLEEP
	fi
done
echo "Failed to check the latest block after $ATTEMPTS attempts"
exit 5 
