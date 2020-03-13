// Geth Tracer that outputs cGLD transfers.
//
// Additional details (e.g. transaction hash & gas used) can be obtained from 
// the block at the corresponding transaction index.

{
  reverted: false,
  transfers: [],

  // fault is invoked when the actual execution of an opcode fails.
  fault(log, db) {
    this.reverted = true;
  },

  // step is invoked for every opcode that the VM executes.
  step(log, db) {
    // Capture any errors immediately
    const error = log.getError();
    if (error !== undefined) {
      this.fault(log, db);
    } else {
      const op = log.op.toString();
      switch (op) {
        case 'CALL':
        case 'CALLCODE':
        case 'DELEGATECALL':
          this.handleCall(log, op);
          break;

        case 'REVERT':
          this.reverted = true;
          break;
      }
    }
  },

  handleCall(log, op) {
    const to = toAddress(log.stack.peek(1).toString(16));
    if (isPrecompiled(to) && toHex(to) == '0x00000000000000000000000000000000000000fd') {
      // This is the transfer precompile "address", inspect its arguments
      const stackOffset = 1;
      const inputOffset = log.stack.peek(2 + stackOffset).valueOf();
      const inputLength = log.stack.peek(3 + stackOffset).valueOf();
      const inputEnd = inputOffset + inputLength;
      const input = toHex(log.memory.slice(inputOffset, inputEnd));
      const transfer = {
        type: 'cGLD transfer precompile',
        from: '0x'+input.slice(2+24, 2+64),
        to: '0x'+input.slice(2+64+24, 2+64*2),
        value: '0x'+input.slice(2+64*2, 2+64*3),
      };
      this.transfers.push(transfer);
    }
  },

  // result is invoked when all the opcodes have been iterated over and returns
  // the final result of the tracing.
  result(ctx, db) {
    if (this.reverted) {
      return []
    }
    if (ctx.type == 'CALL') {
      valueBigInt = bigInt(ctx.value.toString());
      if (valueBigInt.gt(0)) {
        const transfer = {
          type: 'cGLD transfer',
          from: toHex(ctx.from),
          to: toHex(ctx.to),
          value: '0x' + valueBigInt.toString(16),
        };
        this.transfers.unshift(transfer);
      }
    }
    for (var i = 0; i < this.transfers.length; i++) {
      this.transfers[i].block = ctx.block
    }
    return this.transfers
  },
}
