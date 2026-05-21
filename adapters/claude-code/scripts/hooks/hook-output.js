#!/usr/bin/env node
'use strict';

function allow() {
  process.stdout.write('{}\n');
}

function deny(reason) {
  process.stdout.write(JSON.stringify({
    hookSpecificOutput: {
      permissionDecision: 'deny',
      permissionDecisionReason: reason,
    },
  }) + '\n');
}

module.exports = { allow, deny };
