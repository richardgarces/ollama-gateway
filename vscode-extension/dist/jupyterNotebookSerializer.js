"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || (function () {
    var ownKeys = function(o) {
        ownKeys = Object.getOwnPropertyNames || function (o) {
            var ar = [];
            for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k)) ar[ar.length] = k;
            return ar;
        };
        return ownKeys(o);
    };
    return function (mod) {
        if (mod && mod.__esModule) return mod;
        var result = {};
        if (mod != null) for (var k = ownKeys(mod), i = 0; i < k.length; i++) if (k[i] !== "default") __createBinding(result, mod, k[i]);
        __setModuleDefault(result, mod);
        return result;
    };
})();
Object.defineProperty(exports, "__esModule", { value: true });
exports.JupyterNotebookSerializer = void 0;
const vscode = __importStar(require("vscode"));
class JupyterNotebookSerializer {
    // Serializa el notebook a bytes (JSON)
    async serializeNotebook(data, _token) {
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
    async deserializeNotebook(content, _token) {
        const raw = JSON.parse(Buffer.from(content).toString('utf8'));
        const cells = (raw.cells || []).map((cell) => {
            const cellData = new vscode.NotebookCellData(cell.kind, cell.value, cell.language || cell.languageId || 'python');
            if (cell.outputs)
                cellData.outputs = cell.outputs;
            if (cell.metadata)
                cellData.metadata = cell.metadata;
            return cellData;
        });
        return new vscode.NotebookData(cells);
    }
}
exports.JupyterNotebookSerializer = JupyterNotebookSerializer;
