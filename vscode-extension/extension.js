// Wrapper legacy para compatibilidad con Copilot Local
// Solo expone lo fundamental para legacy y registro de refactorización multi-archivo

const registerMultiFileRefactor = require('./multiFileRefactor');

function activate(context) {
  if (typeof registerMultiFileRefactor === 'function') {
    registerMultiFileRefactor(context);
  }
}

function deactivate() {}

// Handler dummy para compatibilidad legacy
async function requestChatCompletion(prompt) {
  return {
    choices: [{ message: { content: 'Legacy fallback: función no implementada.' } }],
  };
}

module.exports = {
  activate,
  deactivate,
  requestChatCompletion,
};
