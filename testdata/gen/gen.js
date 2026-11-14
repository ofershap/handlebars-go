// Generates golden render fixtures from handlebars.js 4.7.8 (pinned).
// Usage: node gen.js > ../goldens.json
const Handlebars = require('handlebars');
const corpus = require('./corpus.json');

const out = [];
for (const c of corpus) {
  if (c.skipReason) continue;
  const h = Handlebars.create();
  if (c.partials) for (const [n, src] of Object.entries(c.partials)) h.registerPartial(n, src);
  if (c.helpers) {
    for (const [n, spec] of Object.entries(c.helpers)) {
      if (spec === '__param0__') h.registerHelper(n, function (x) { return x; });
      else if (spec === '__hash__') h.registerHelper(n, function (opts) { return opts.hash.first + '-' + opts.hash.last; });
      else if (spec === '__block__') h.registerHelper(n, function () { const o = arguments[arguments.length - 1]; return 'B[' + o.fn(this) + ']'; });
      else if (spec === 'caught') h.registerHelper(n, function () { return 'caught'; });
      else h.registerHelper(n, function () { return spec; });
    }
  }
  const entry = { name: c.name, template: c.template, context: c.context, options: c.options || {}, partials: c.partials || {}, helpers: c.helpers || {} };
  try {
    const t = h.compile(c.template, c.options || {});
    entry.expected = t(c.context, c.options || {});
  } catch (e) {
    if (c.expectError) { entry.expectError = true; entry.error = String(e.message || e); }
    else { entry.genError = String(e.message || e); }
  }
  out.push(entry);
}
console.log(JSON.stringify({ generator: 'handlebars@' + Handlebars.VERSION, cases: out }, null, 1));
