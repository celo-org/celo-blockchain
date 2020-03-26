// Geth Tracer that outputs cGLD transfers.
//
// Additional details (e.g. transaction hash & gas used) can be obtained from 
// the block at the corresponding transaction index.

{
  callStack: [ { transfers: [] } ],
  assertionFailed: false,
  statusRevert: 'revert',
  statusSuccess: 'success',

  topCall() {
    return this.callStack[this.callStack.length - 1];
  },

  pushTransfers(transfers, call, transferStatus) {
    for (var index in call.transfers) {
      const transfer = call.transfers[index];
      // Successful transfers become reverted if any ancestor call reverts.
      if (transfer.status != this.statusRevert) {
        transfer.status = transferStatus;
      }
      transfers.push(transfer);
    }
  },

  // fault() is invoked when the actual execution of an opcode fails.
  fault(log, db) {
    if (this.callStack.length != log.getDepth()) {
      this.assertionFailed = true;
    }

    const failedCall = this.callStack.pop();
    // Revert all transfers that are descendents of the failed call.
    this.pushTransfers(this.topCall().transfers, failedCall, this.statusRevert);
  },

  // step() is invoked for every opcode that the VM executes.
  step(log, db) {
    const depth = log.getDepth()

    if (this.callStack.length - 1 == depth) {
      const successfulCall = this.callStack.pop();
      // Propogate transfers made during the successful call.
      this.pushTransfers(this.topCall().transfers, successfulCall, this.statusSuccess);
    }

    if (this.callStack.length != depth) {
      this.assertionFailed = true;
    }

    // Capture any errors immediately.
    const error = log.getError();
    if (error !== undefined) {
      this.fault(log, db);
    } else {
      const op = log.op.toString();
      switch (op) {
        case 'CREATE':
        case 'CREATE2':
          this.callStack.push({ transfers: [] })
          this.handleCreate(log, op);
          break;

        case 'SELFDESTRUCT':
          this.callStack.push({ transfers: [] })
          this.handleDestruct(log, db);
          break;

        case 'CALL':
        case 'CALLCODE':
        case 'STATICCALL':
        case 'DELEGATECALL':
          this.callStack.push({ transfers: [] })
          if (op != 'STATICCALL') {
            this.handleCall(log, op);
          }
          break;
      }
    }
  },
  
  handleCreate(log, op) {
    valueBigInt = bigInt(log.stack.peek(0));
    if (valueBigInt.gt(0)) {
      this.topCall().transfers.push({
        type: 'cGLD create contract transfer',
        to: toHex(log.contract.getAddress()),
        value: '0x' + valueBigInt.toString(16),
      });
    }
  },

  handleDestruct(log, db) {
    const contractAddress = log.contract.getAddress();
    const valueBigInt = db.getBalance(contractAddress)
    if (valueBigInt.gt(0)) {
      this.topCall().transfers.push({
        type: 'cGLD destroy contract transfer',
        from: toHex(contractAddress),
        to: toHex(toAddress(log.stack.peek(0).toString(16))),
        value: '0x' + valueBigInt.toString(16),
      });
    }
  },

  handleCall(log, op) {
    const to = toAddress(log.stack.peek(1).toString(16));
    if (!isPrecompiled(to)) {
      if (op != 'DELEGATECALL') {
        valueBigInt = bigInt(log.stack.peek(2));
        if (valueBigInt.gt(0)) {
          this.topCall().transfers.push({
            type: 'cGLD nested transfer',
            from: toHex(log.contract.getAddress()),
            to: toHex(to),
            value: '0x' + valueBigInt.toString(16),
          });
        }
      }
    } else if (toHex(to) == '0x00000000000000000000000000000000000000fd') {
      // This is the transfer precompile "address", inspect its arguments.
      const stackOffset = 1;
      const inputOffset = log.stack.peek(2 + stackOffset).valueOf();
      const inputLength = log.stack.peek(3 + stackOffset).valueOf();
      const inputEnd = inputOffset + inputLength;
      const input = toHex(log.memory.slice(inputOffset, inputEnd));

      this.topCall().transfers.push({
        type: 'cGLD transfer precompile',
        from: '0x'+input.slice(2+24, 2+64),
        to: '0x'+input.slice(2+64+24, 2+64*2),
        value: '0x'+input.slice(2+64*2, 2+64*3),
      });
    }
  },

  // result() is invoked when all the opcodes have been iterated over and returns
  // the final result of the tracing.
  result(ctx, db) {
    const transfers = []

    if (this.callStack.length != 1) {
      this.assertionFailed = true;
    } else {
      if (ctx.type == 'CALL') {
        valueBigInt = bigInt(ctx.value.toString());
        if (valueBigInt.gt(0)) {
          transfers.push({
            type: 'cGLD transfer',
            from: toHex(ctx.from),
            to: toHex(ctx.to),
            value: '0x' + valueBigInt.toString(16),
            status: this.statusSuccess,
          });
        }
      }
      this.pushTransfers(transfers, this.callStack.pop(), this.statusSuccess);
    }

    // Return in same format as callTracer: -calls, +transfers, +block, and +status.
    return {
      type:      ctx.type,
      from:      toHex(ctx.from),
      to:        toHex(ctx.to),
      value:     '0x' + ctx.value.toString(16),
      gas:       '0x' + bigInt(ctx.gas).toString(16),
      gasUsed:   '0x' + bigInt(ctx.gasUsed).toString(16),
      input:     toHex(ctx.input),
      output:    toHex(ctx.output),
      block:     ctx.block,
      time:      ctx.time,
      status:    this.assertionFailed ? 'assertionFailed' : this.statusSuccess,
      transfers: transfers,
    };
  },
}
