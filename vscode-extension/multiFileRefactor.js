// Función: Refactorización multi-archivo
// Extraída de extension.js para modularidad y depuración

const vscode = require('vscode');

module.exports = function registerMultiFileRefactor(context) {
  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.multiFileRefactor', async (fileRanges) => {
    // fileRanges: [{ uri, range }]
    if (!Array.isArray(fileRanges) || fileRanges.length === 0) {
      vscode.window.showWarningMessage('No files/ranges provided for multi-file refactor.');
      return;
    }
    const edits = [];
    for (const { uri, range } of fileRanges) {
      const doc = await vscode.workspace.openTextDocument(uri);
      const code = doc.getText(range);
      const lang = doc.languageId || 'plaintext';
      const prompt = 'Refactor the following ' + lang + ' code for clarity and maintainability. Return ONLY the refactored code.\n\n' +
        'Code:\n```' + lang + '\n' + code + '\n```';
      const res = await requestChatCompletion(prompt, undefined, undefined);
      let refactored = String(res?.content || res?.completion || '').trim();
      if (refactored.startsWith('```')) {
        const lines = refactored.split('\n');
        lines.shift();
        if (lines.length > 0 && lines[lines.length - 1].trim() === '```') lines.pop();
        refactored = lines.join('\n');
      }
      if (refactored && refactored !== code) {
        edits.push({ uri, range, newText: refactored });
      }
    }
    if (edits.length === 0) {
      vscode.window.showInformationMessage('No refactorings suggested.');
      return;
    }
    // Preview: mostrar diff para cada archivo
    for (const edit of edits) {
      const doc = await vscode.workspace.openTextDocument(edit.uri);
      const originalDoc = await vscode.workspace.openTextDocument({ content: doc.getText(edit.range), language: doc.languageId });
      const proposedDoc = await vscode.workspace.openTextDocument({ content: edit.newText, language: doc.languageId });
      await vscode.commands.executeCommand('vscode.diff', originalDoc.uri, proposedDoc.uri, 'Multi-file Refactor Preview', { preview: true, viewColumn: vscode.ViewColumn.Beside });
    }
    const decision = await vscode.window.showInformationMessage('Apply all refactorings?', 'Apply', 'Discard');
    if (decision === 'Apply') {
      const wsEdit = new vscode.WorkspaceEdit();
      for (const edit of edits) {
        wsEdit.replace(edit.uri, edit.range, edit.newText);
      }
      const ok = await vscode.workspace.applyEdit(wsEdit);
      if (ok) vscode.window.showInformationMessage('Multi-file refactorings applied.');
    }
  }));
}
