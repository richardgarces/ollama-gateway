import * as vscode from 'vscode';

export class JupyterNotebookSerializer implements vscode.NotebookSerializer {
  // Serializa el notebook a bytes (JSON)
  async serializeNotebook(data: vscode.NotebookData, _token: vscode.CancellationToken): Promise<Uint8Array> {
    // Simple passthrough: convierte NotebookData a JSON
    const raw = {
      cells: data.cells.map(cell => ({
        kind: cell.kind,
        language: cell.languageId,
        value: cell.value,
        outputs: cell.outputs,
        metadata: cell.metadata,
      })),
      metadata: data.metadata,
    };
    return Buffer.from(JSON.stringify(raw), 'utf8');
  }

  // Deserializa bytes (JSON) a NotebookData
  async deserializeNotebook(content: Uint8Array, _token: vscode.CancellationToken): Promise<vscode.NotebookData> {
    const raw = JSON.parse(Buffer.from(content).toString('utf8'));
    const cells = (raw.cells || []).map((cell: any) => {
      const cellData = new vscode.NotebookCellData(
        cell.kind,
        cell.value,
        cell.language || cell.languageId || 'python'
      );
      if (cell.outputs) cellData.outputs = cell.outputs;
      if (cell.metadata) cellData.metadata = cell.metadata;
      return cellData;
    });
    return new vscode.NotebookData(cells);
  }
}
