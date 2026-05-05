'use strict';

const path = require('path');
const { writeState, readAgentState } = require(
  path.resolve(__dirname, '..', '..', 'claude-code', 'packages', 'agent-state'),
);

const AGENT_ID = 'pi';

function writePiState(sessionId, update, agentsDir) {
  const merged = {
    ...update,
    session_id: sessionId,
    agent_id: AGENT_ID,
  };
  writeState(sessionId, merged, agentsDir);
}

function readPiState(sessionId, agentsDir) {
  return readAgentState(sessionId, agentsDir);
}

module.exports = { writePiState, readPiState, AGENT_ID };
