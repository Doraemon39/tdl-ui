const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const ts = require('typescript')

function loadTs(relative, overrides = {}, cache = new Map()) {
  const file = path.resolve(__dirname, '..', 'src', relative)
  if (cache.has(file)) return cache.get(file)
  const source = fs.readFileSync(file, 'utf8')
  const output = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  const module = { exports: {} }
  cache.set(file, module.exports)
  const localRequire = id => {
    if (Object.hasOwn(overrides, id)) return overrides[id]
    if (id.startsWith('.')) return loadTs(path.relative(path.resolve(__dirname, '..', 'src'), path.resolve(path.dirname(file), `${id}.ts`)), overrides, cache)
    return require(id)
  }
  const wrapper = vm.runInThisContext(`(function(require,module,exports){${output}\n})`, { filename: file })
  wrapper(localRequire, module, module.exports)
  return module.exports
}
module.exports = { loadTs }
