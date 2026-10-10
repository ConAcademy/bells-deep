// Starts the go-booba terminal and the Go WASM program. Relative URLs keep the
// page working at / and under the GitHub Pages /bells-deep/ prefix.
const status = document.querySelector('#status');
let terminal;
let failed = false;
function fail(error) {
  failed = true;
  console.error(error);
  status.textContent = `bells-deep could not start.\n${error.message || error}\nReload this page to try again.`;
  status.hidden = false;
}

try {
  if (!navigator.gpu) {
    console.warn('No WebGPU: bells-deep falls back to the CPU analytic preview.');
  }
  const { BoobaTerminal } = await import('./_assets/booba/booba.js');
  const go = new Go();
  const response = await fetch(new URL('app.wasm', location.href));
  if (!response.ok) throw new Error(`WebAssembly download: HTTP ${response.status}`);
  const result = await WebAssembly.instantiateStreaming(response, go.importObject);
  terminal = new BoobaTerminal('terminal');
  // Terminal initialization creates the Kitty shared-memory registry.
  await terminal.init();
  go.run(result.instance).catch(fail);
  terminal.connectWasm();
  if (!failed) status.hidden = true;
  terminal.focus();
} catch (error) {
  fail(error);
}
