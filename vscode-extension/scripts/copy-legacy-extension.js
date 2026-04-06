// Copia extension.js a la raíz del paquete de salida para compatibilidad legacy
const fs = require('fs');
const path = require('path');

const src = path.resolve(__dirname, '../extension.js');
const dest = path.resolve(__dirname, '../dist/../extension.js');

if (fs.existsSync(src)) {
  fs.copyFileSync(src, dest);
  console.log('extension.js copiado a la raíz del paquete de salida.');
} else {
  console.error('No se encontró extension.js para copiar.');
}
