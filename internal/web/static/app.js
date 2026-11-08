'use strict';
/* Revenant UI. Plain DOM, no framework, no innerHTML: every string from the
   server goes in through textContent, so a hostile source cannot inject markup. */

const NS = 'http://www.w3.org/2000/svg';
const $ = (sel, root = document) => root.querySelector(sel);

function h(tag, props, ...kids) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(props || {})) {
    if (v == null || v === false) continue;
    if (k === 'class') el.className = v;
    else if (k.startsWith('on')) el.addEventListener(k.slice(2), v);
    else el.setAttribute(k, v === true ? '' : v);
  }
  for (const kid of kids.flat(Infinity)) {
    if (kid == null || kid === false) continue;
    el.append(kid.nodeType ? kid : document.createTextNode(String(kid)));
  }
  return el;
}

function icon(name, fill) {
  const s = document.createElementNS(NS, 'svg');
  s.setAttribute('class', fill ? 'ic fill' : 'ic');
  s.setAttribute('aria-hidden', 'true');
  const u = document.createElementNS(NS, 'use');
  u.setAttribute('href', '#i-' + name);
  s.append(u);
  return s;
}

async function api(path, body) {
  const opt = body === undefined ? {} : { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) };
  let res;
  try { res = await fetch('/api' + path, opt); } catch { throw new Error('Cannot reach the Revenant server.'); }
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

let toastTimer;
function say(msg, bad) {
  const box = $('#toast');
  box.replaceChildren(h('p', { class: bad ? 'bad' : '' }, msg));
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => box.replaceChildren(), 6000);
}

// Run a handler; show its error instead of letting it vanish.
const guarded = fn => async (...a) => { try { await fn(...a); } catch (e) { say(e.message, true); } };

const V = {
  verbatim: { label: 'Verbatim', icon: 'check', note: 'Word for word in the source.' },
  drifted: { label: 'Drifted', icon: 'wave', note: 'Found in the source, but the words differ.' },
  unsupported: { label: 'Unsupported', icon: 'x', note: 'Not found in any loaded source. That does not prove it is false: the real source may simply not be loaded.' },
  'no-source': { label: 'No source', icon: 'question', note: 'No source documents are loaded, so there is nothing to check against.' },
};
const LOC = { intact: 'check', moved: 'wave', drifted: 'wave', orphaned: 'x', redacted: 'x', unreadable: 'x' };
const LOC_NOTE = {
  intact: 'Same words, same place.',
  moved: 'Same words, now elsewhere in the source.',
  drifted: 'The source changed under this citation.',
  orphaned: 'Nothing close enough is left in the source.',
  redacted: 'The source text was removed from the store.',
  unreadable: 'The stored source could not be read.',
};

const vclass = v => 'v-' + v;
function chip(kind, text, extra) {
  return h('span', { class: `chip tone-${kind} ${extra || ''}` }, icon(V[kind]?.icon || LOC[kind] || 'question'), text);
}
const pct = n => Math.round(n / 10) + '%';
const when = ts => new Date(ts).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });

function diffView(ops) {
  const p = h('p', { class: 'diff', 'aria-label': 'Word by word comparison' });
  ops.forEach((o, i) => {
    if (i) p.append(' ');
    if (o.k === 'eq') p.append(o.q);
    else if (o.k === 'sub') p.append(h('del', { title: 'in the quote' }, o.q), ' ', h('ins', { title: 'in the source' }, o.s));
    else if (o.k === 'del') p.append(h('del', { title: 'only in the quote' }, o.q));
    else p.append(h('ins', { title: 'only in the source' }, o.s));
  });
  const f = document.createDocumentFragment(); // a fragment, not an array: append() would print an array as text
  f.append(p, h('p', { class: 'legend' }, h('del', {}, 'struck'), ' = only in the quote.  ', h('ins', {}, 'underlined'), ' = only in the source.'));
  return f;
}

// Code-point helpers: the server counts runes, JS strings count UTF-16 units.
const cps = s => Array.from(s);
function selectionRange(container) {
  const sel = getSelection();
  if (!sel.rangeCount || sel.isCollapsed) return null;
  const r = sel.getRangeAt(0);
  if (!container.contains(r.commonAncestorContainer)) return null;
  const pre = document.createRange();
  pre.selectNodeContents(container);
  pre.setEnd(r.startContainer, r.startOffset);
  const a = cps(pre.toString()).length;
  return [a, a + cps(r.toString()).length];
}

/* ---------------- shared state ---------------- */

const state = { text: '', found: null, counts: null, sel: -1, editing: true, docs: [] };

async function refreshStatus() {
  try {
    const s = await api('/status');
    state.docs = await api('/docs');
    $('#status').textContent = `${s.docs} source${s.docs === 1 ? '' : 's'} · ${s.events} log events · key ${s.key}`;
  } catch { /* the page still works without the status line */ }
}

/* ---------------- séance ---------------- */

function viewSeance() {
  const root = h('div', { class: 'view' },
    h('h1', { tabindex: '-1' }, 'Séance'),
    h('p', { class: 'lede' }, 'Paste a draft. Every passage in double quotes (four words or more) is called back to its source and graded. Click a quotation to see what the source really says.'));
  const sheetBox = h('div', {});
  const margin = h('aside', { class: 'panel margin', 'aria-label': 'Findings' });
  root.append(h('div', { class: 'seance' }, h('div', {}, sheetBox), margin));

  let ta;
  const run = guarded(async () => {
    if (ta) state.text = ta.value;
    if (!state.text.trim()) return say('Paste a draft first.', true);
    const r = await api('/check-text', { text: state.text });
    if (!r.found.length) return say('No quoted passages of four words or more were found.', true);
    Object.assign(state, { found: r.found, counts: r.counts, editing: false });
    state.sel = r.found.findIndex(f => f.result.verdict !== 'verbatim');
    drawSheet(); drawMargin();
  });
  const sample = guarded(async () => {
    const d = await api('/demo', {});
    state.text = d.report;
    if (ta) ta.value = d.report; // run() reads the textarea
    await refreshStatus();
    await run();
    say('Sample sources loaded: the Gettysburg Address and the Declaration of Independence.');
  });

  function drawSheet() {
    const bar = h('div', { class: 'sheet-bar' },
      h('button', { class: 'btn primary', type: 'button', onclick: run }, icon('seal'), state.editing ? 'Check quotes' : 'Check again'),
      state.editing ? null : h('button', { class: 'btn', type: 'button', onclick: () => { state.editing = true; drawSheet(); } }, 'Edit draft'),
      h('button', { class: 'btn', type: 'button', onclick: sample }, 'Load sample'));
    let body;
    if (state.editing || !state.found) {
      ta = h('textarea', { class: 'draft', id: 'draft', 'aria-label': 'Draft to check', spellcheck: 'false', placeholder: 'Paste a draft here, or load the sample to see how it works…' });
      ta.value = state.text;
      ta.addEventListener('input', () => { state.text = ta.value; });
      body = ta;
    } else {
      ta = null;
      body = h('div', { class: 'sheet-text', id: 'annotated' }, annotated());
    }
    sheetBox.replaceChildren(h('section', { class: 'sheet', 'aria-label': 'Draft' }, bar, body));
  }

  function annotated() {
    const chars = cps(state.text);
    const frag = document.createDocumentFragment();
    let pos = 0;
    state.found.forEach((f, i) => {
      frag.append(chars.slice(pos, f.start).join(''));
      const r = f.result, v = r.verdict, mat = r.severity === 'material';
      const label = V[v].label + (v === 'drifted' && mat ? ' · material' : '');
      const q = h('span', {
        class: `q ${vclass(v)}${mat ? ' mat' : ''}`, role: 'button', tabindex: '0', 'aria-pressed': String(i === state.sel),
        'aria-label': `Quotation ${i + 1}: ${label}`,
        onclick: () => pick(i),
        onkeydown: e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); pick(i); } },
      }, chars.slice(f.start, f.end).join(''));
      frag.append(q, chip(v, label, mat ? 'mat' : ''));
      pos = f.end;
    });
    frag.append(chars.slice(pos).join(''));
    return frag;
  }

  function pick(i) {
    state.sel = i;
    root.querySelectorAll('.q').forEach((el, k) => el.setAttribute('aria-pressed', String(k === i)));
    drawMargin();
  }

  function drawMargin() {
    margin.replaceChildren(h('h2', {}, 'Findings'));
    if (!state.found) {
      margin.append(
        h('p', { class: 'explain' }, 'Each quotation gets one of four verdicts. The mark in the text and the label beside it always agree, so nothing depends on colour.'),
        h('ul', { class: 'tally' }, Object.keys(V).map(k => h('li', { class: `tone-${k}` }, icon(V[k].icon), h('span', { class: 'lbl' }, V[k].label)))),
        state.docs.length ? null : h('p', { class: 'alert' }, icon('question'), ' No sources are loaded yet. Load the sample, or add your own in the Archive.'));
      return;
    }
    const c = state.counts || {};
    margin.append(h('ul', { class: 'tally' }, Object.keys(V).map(k =>
      h('li', { class: `tone-${k}` }, icon(V[k].icon), h('span', { class: 'lbl' }, V[k].label), h('b', {}, c[k] || 0)))));
    if (c.material) margin.append(h('p', { class: 'alert', role: 'note' }, icon('wave'), ` ${c.material} quotation${c.material === 1 ? '' : 's'} changed a word that can flip the meaning (a negation, number, name or modal).`));
    const f = state.found[state.sel];
    margin.append(f ? detail(f) : h('p', { class: 'explain' }, 'Every quotation is verbatim. Select one to see where it sits in its source.'));
  }

  function detail(f) {
    const r = f.result, v = r.verdict;
    const out = h('div', { class: 'sec' },
      h('h3', { class: `verdict-h tone-${v}` }, icon(V[v].icon), V[v].label, r.severity ? h('span', { class: 'sev' }, r.severity) : null));
    out.append(h('p', { class: 'explain' }, V[v].note));
    if (v === 'unsupported' || v === 'no-source') {
      out.append(h('a', { class: 'btn', href: '#/archive' }, 'Open the archive'));
      return out;
    }
    out.append(h('div', { class: 'simrow' }, 'Match', h('progress', { max: '1000', value: String(r.sim), 'aria-label': 'Similarity to the source' }), h('b', {}, pct(r.sim))));
    const sp = r.spans || [];
    sp.forEach((s, k) => out.append(h('blockquote', { class: 'src' }, s.excerpt,
      h('cite', {}, `${r.doc} · version ${r.ver} · characters ${s.start}–${s.end}${sp.length > 1 ? ` (piece ${k + 1})` : ''}`))));
    if (r.ops && r.ops.some(o => o.k !== 'eq')) out.append(h('h3', {}, 'What changed'), diffView(r.ops));
    if (r.notes && r.notes.length) out.append(h('ul', { class: 'notes' }, r.notes.map(n => h('li', {}, n))));
    out.append(citeForm(f), sealForm(f), trailForm(f));
    return out;
  }

  function citeForm(f) {
    const r = f.result;
    const label = h('input', { type: 'text', value: 'draft', maxlength: '200', id: 'cite-label' });
    const mode = h('select', { id: 'cite-mode' }, h('option', { value: 'follow' }, 'Follow the latest version'), h('option', { value: 'pinned' }, 'Pin to this version'));
    return h('details', { class: 'act' }, h('summary', {}, icon('link'), 'Cite this passage'),
      h('div', { class: 'body' },
        h('p', { class: 'hint' }, 'Records a pointer to this exact span, not a copy. If the source is edited later, the citation shows the change.'),
        h('label', { class: 'field' }, h('span', {}, 'Where is it used?'), label),
        h('label', { class: 'field' }, h('span', {}, 'Track'), mode),
        h('button', { class: 'btn', type: 'button', onclick: guarded(async () => {
          const c = await api('/cites', { label: label.value || 'draft', doc: r.doc, ver: r.ver, quote: f.text, mode: mode.value });
          say(`Cited ${c.doc} v${c.ver}. Open the Archive to see who haunts it.`);
          refreshStatus();
        }) }, 'Record citation')));
  }

  function sealForm(f) {
    const r = f.result;
    const box = h('div', {});
    return h('details', { class: 'act' }, h('summary', {}, icon('seal'), 'Seal a receipt'),
      h('div', { class: 'body' },
        h('p', { class: 'hint' }, 'A signed record of this check. Anyone can verify it offline. It proves what this key saw, not that the source is authentic.'),
        h('button', { class: 'btn', type: 'button', onclick: guarded(async () => {
          const d = await api('/receipts', { quote: f.text, doc: r.doc, ver: r.ver });
          const json = JSON.stringify(d.receipt, null, 2);
          const area = h('textarea', { class: 'receipt-box', readonly: true, 'aria-label': 'Receipt JSON' });
          area.value = json;
          const link = h('a', { class: 'btn', download: `receipt-${d.id.slice(0, 12)}.json`, href: URL.createObjectURL(new Blob([json], { type: 'application/json' })) }, icon('down'), 'Download');
          box.replaceChildren(h('p', { class: 'hint wrap' }, 'Receipt ', h('code', {}, d.id.slice(0, 16) + '…')), area, h('div', { class: 'row' }, link));
          refreshStatus();
        }) }, 'Seal it'), box));
  }

  function trailForm(f) {
    const r = f.result;
    const body = h('div', { class: 'body' });
    const det = h('details', { class: 'act' }, h('summary', {}, icon('link'), 'Add to a trail'), body);
    det.addEventListener('toggle', guarded(async () => {
      if (!det.open || body.childElementCount) return;
      const trails = await api('/trails');
      const sel = h('select', { id: 'trail-pick' }, trails.map(t => h('option', { value: t.id }, t.title)), h('option', { value: '' }, 'New trail…'));
      const title = h('input', { type: 'text', maxlength: '200', placeholder: 'Title of the new trail' });
      const claim = h('input', { type: 'text', maxlength: '500', placeholder: 'The claim this quotation backs' });
      const sync = () => { title.parentElement.hidden = sel.value !== ''; };
      sel.addEventListener('change', sync);
      body.append(
        h('p', { class: 'hint' }, 'A trail is a path from claims to the exact source versions behind them. Each step carries a signed receipt.'),
        h('label', { class: 'field' }, h('span', {}, 'Trail'), sel),
        h('label', { class: 'field' }, h('span', {}, 'New trail title'), title),
        h('label', { class: 'field' }, h('span', {}, 'Claim'), claim),
        h('button', { class: 'btn', type: 'button', onclick: guarded(async () => {
          let id = sel.value;
          if (!id) id = (await api('/trails', { title: title.value })).id;
          await api(`/trails/${id}/steps`, { claim: claim.value, quote: f.text, doc: r.doc, ver: r.ver });
          say('Step added. Open Trails to replay it.');
          location.hash = '#/trails/' + id;
        }) }, 'Add step'));
      sync();
    }));
    return det;
  }

  drawSheet(); drawMargin();
  return root;
}

/* ---------------- archive ---------------- */

async function viewArchive(id, ver) {
  const docs = await api('/docs');
  state.docs = docs;
  const root = h('div', { class: 'view' },
    h('h1', { tabindex: '-1' }, 'Archive'),
    h('p', { class: 'lede' }, 'Sources are never overwritten. Saving an edit adds a new version, and every citation then reports whether its passage survived.'));

  const list = h('ul', { class: 'doclist' }, docs.map(d => h('li', {}, h('a', { href: '#/archive/' + d.id, 'aria-current': String(d.id === id) },
    d.title, h('small', {}, `${d.id} · ${d.versions} version${d.versions === 1 ? '' : 's'} · ${d.runes.toLocaleString()} characters`)))));
  const side = h('div', {}, h('h2', {}, 'Sources'), docs.length ? list : h('p', { class: 'hint' }, 'None yet.'), addForm());
  const main = h('div', {});
  root.append(h('div', { class: 'split' }, side, main));

  if (!docs.length) {
    main.append(h('div', { class: 'empty' }, h('h2', {}, 'The archive is empty'), h('p', {}, 'Add a source on the left, or load two public-domain texts to try things out.'),
      h('div', { class: 'row' }, h('button', { class: 'btn primary', type: 'button', onclick: guarded(async () => { await api('/demo', {}); await refreshStatus(); say('Sample sources loaded.'); route(); }) }, 'Load sample sources'))));
    return root;
  }
  if (!id) id = docs[0].id;
  const [d, versions, cites] = await Promise.all([
    api(`/docs/${encodeURIComponent(id)}${ver ? '?ver=' + ver : ''}`),
    api(`/docs/${encodeURIComponent(id)}/versions`),
    api('/cites?doc=' + encodeURIComponent(id)),
  ]);
  side.querySelectorAll('.doclist a').forEach(a => a.setAttribute('aria-current', String(a.getAttribute('href') === '#/archive/' + id)));

  const pick = h('select', { id: 'ver', 'aria-label': 'Version' }, versions.map(v => h('option', { value: v.ver, selected: v.ver === d.ver }, `Version ${v.ver} · ${when(v.ts)}`)));
  pick.addEventListener('change', () => { location.hash = `#/archive/${id}/${pick.value}`; });
  const reader = h('div', { class: 'reader', id: 'reader', tabindex: '0', 'aria-label': `Text of ${d.title}, version ${d.ver}. Select a passage to ask who quotes it.` }, highlighted(d, cites));
  const haunt = h('button', { class: 'btn', type: 'button', disabled: true }, 'Who haunts this passage?');
  const results = h('div', { class: 'sec', 'aria-live': 'polite' });
  let range = null;
  document.addEventListener('selectionchange', () => {
    if (!reader.isConnected) return;
    const r = selectionRange(reader);
    if (r) range = r;
    haunt.disabled = !range;
  });
  haunt.addEventListener('click', guarded(async () => {
    if (!range) return;
    const hs = await api(`/haunts?doc=${encodeURIComponent(id)}&ver=${d.ver}&start=${range[0]}&end=${range[1]}`);
    results.replaceChildren(h('h2', {}, `Who haunts characters ${range[0]}–${range[1]}`),
      ...(hs.length ? hs.map(citeCard) : [h('p', { class: 'explain' }, 'Nothing quotes this passage yet.')]));
  }));

  const editBox = h('div', {});
  const editBtn = h('button', { class: 'btn', type: 'button', onclick: () => {
    const ta = h('textarea', { id: 'edit-text', 'aria-label': 'New text of the source' });
    ta.value = d.text;
    editBox.replaceChildren(h('div', { class: 'panel' },
      h('p', { class: 'hint' }, d.ver < versions.length ? `You are editing version ${d.ver}, not the latest. The result becomes version ${versions.length + 1}.` : `This saves as version ${versions.length + 1}. Version ${d.ver} stays exactly as it is.`),
      h('label', { class: 'field' }, h('span', {}, 'Source text'), ta),
      h('div', { class: 'row' },
        h('button', { class: 'btn primary', type: 'button', onclick: guarded(async () => {
          const v = await api('/docs', { id, title: d.title, text: ta.value });
          await refreshStatus();
          say(`Saved as version ${v.ver}. Citations have been re-checked.`);
          location.hash = '#/archive/' + id;
          if (location.hash === '#/archive/' + id) route();
        }) }, 'Save as new version'),
        h('button', { class: 'btn quiet', type: 'button', onclick: () => editBox.replaceChildren() }, 'Cancel'))));
    $('#edit-text').focus();
  } }, 'Write a new version');

  main.append(
    h('div', { class: 'reader-head' }, h('h2', {}, d.title), h('label', { class: 'field' }, h('span', {}, 'Version'), pick), editBtn, haunt),
    editBox,
    h('div', { class: 'sheet' }, reader),
    results,
    h('div', { class: 'sec' }, h('h2', {}, `Citations of ${d.title}`),
      cites.length ? cites.map(citeCard) : h('p', { class: 'explain' }, 'No citations yet. Check a draft in the Séance, open a quotation and choose “Cite this passage”.')));
  return root;
}

function highlighted(d, cites) {
  const chars = cps(d.text);
  const spans = cites.filter(c => c.at_ver === d.ver && ['intact', 'moved', 'drifted'].includes(c.status) && c.at_end > c.at_start)
    .sort((a, b) => a.at_start - b.at_start);
  const frag = document.createDocumentFragment();
  let pos = 0;
  for (const c of spans) {
    if (c.at_start < pos) continue; // overlapping citations: the first one wins the highlight
    frag.append(chars.slice(pos, c.at_start).join(''));
    frag.append(h('span', { class: 'cited' + (c.status === 'drifted' ? ' drifted' : ''), title: `${c.label} (${c.status})` }, chars.slice(c.at_start, c.at_end).join('')));
    pos = c.at_end;
  }
  frag.append(chars.slice(pos).join(''));
  return frag;
}

function citeCard(c) {
  const card = h('article', { class: 'cite' },
    h('header', {}, chip(c.status, c.status[0].toUpperCase() + c.status.slice(1)), h('strong', {}, c.label),
      h('span', { class: 'pill' }, c.mode === 'pinned' ? `pinned to v${c.ver}` : `follows latest · cited at v${c.ver}`)),
    h('p', { class: 'explain' }, LOC_NOTE[c.status] || ''),
    h('blockquote', { class: 'src' }, c.exact, h('cite', {}, `cited text · ${c.doc} v${c.ver} · characters ${c.start}–${c.end}`)));
  if (c.status === 'drifted') {
    card.append(h('p', { class: 'hint' }, `Now, in version ${c.at_ver} (${pct(c.sim)} alike):`), h('blockquote', { class: 'src' }, c.now));
    if (c.ops && c.ops.some(o => o.k !== 'eq')) card.append(diffView(c.ops));
  } else if (c.status === 'moved') {
    card.append(h('p', { class: 'hint' }, `Now at characters ${c.at_start}–${c.at_end} of version ${c.at_ver}.`));
  }
  return card;
}

function addForm() {
  const id = h('input', { type: 'text', id: 'new-id', maxlength: '64', placeholder: 'e.g. gettysburg', autocomplete: 'off', pattern: '[a-z0-9][a-z0-9._-]*' });
  const title = h('input', { type: 'text', id: 'new-title', maxlength: '200', autocomplete: 'off' });
  const text = h('textarea', { id: 'new-text' });
  return h('details', { class: 'act sec' }, h('summary', {}, 'Add a source'),
    h('div', { class: 'body' },
      h('label', { class: 'field' }, h('span', {}, 'Id (lowercase, no spaces)'), id),
      h('label', { class: 'field' }, h('span', {}, 'Title'), title),
      h('label', { class: 'field' }, h('span', {}, 'Text'), text),
      h('p', { class: 'hint' }, 'Reusing an id adds a new version of that source.'),
      h('button', { class: 'btn primary', type: 'button', onclick: guarded(async () => {
        const v = await api('/docs', { id: id.value.trim(), title: title.value.trim(), text: text.value });
        await refreshStatus();
        say(`${v.id} is at version ${v.ver}.`);
        location.hash = '#/archive/' + v.id;
        route();
      }) }, 'Save source')));
}

/* ---------------- trails ---------------- */

async function viewTrails(id) {
  const trails = await api('/trails');
  const root = h('div', { class: 'view' },
    h('h1', { tabindex: '-1' }, 'Trails'),
    h('p', { class: 'lede' }, 'A trail walks from each claim to the exact source version behind it, like Vannevar Bush’s Memex trails, except every step is sealed. Replay a trail to see whether the sources still say what they said.'));
  const list = h('ul', { class: 'doclist' }, trails.map(t => h('li', {}, h('a', { href: '#/trails/' + t.id, 'aria-current': String(t.id === id) },
    t.title, h('small', {}, `${t.steps.length} step${t.steps.length === 1 ? '' : 's'}`)))));
  const title = h('input', { type: 'text', maxlength: '200', id: 'trail-title' });
  const side = h('div', {}, h('h2', {}, 'Trails'), trails.length ? list : h('p', { class: 'hint' }, 'None yet.'),
    h('details', { class: 'act sec' }, h('summary', {}, 'New trail'), h('div', { class: 'body' },
      h('label', { class: 'field' }, h('span', {}, 'Title'), title),
      h('button', { class: 'btn primary', type: 'button', onclick: guarded(async () => {
        const t = await api('/trails', { title: title.value });
        location.hash = '#/trails/' + t.id;
      }) }, 'Create'))));
  const main = h('div', {});
  root.append(h('div', { class: 'split' }, side, main));
  if (!trails.length) {
    main.append(h('div', { class: 'empty' }, h('h2', {}, 'No trails yet'), h('p', {}, 'In the Séance, open a quotation and choose “Add to a trail”.')));
    return root;
  }
  const t = trails.find(x => x.id === id) || trails[trails.length - 1];
  side.querySelectorAll('.doclist a').forEach(a => a.setAttribute('aria-current', String(a.getAttribute('href') === '#/trails/' + t.id)));
  const out = h('div', {});
  const draw = replay => {
    const steps = replay || t.steps.map(s => ({ ...s }));
    out.replaceChildren(steps.length ? h('ol', { class: 'thread' }, steps.map((s, i) => knot(s, i, !!replay))) : h('p', { class: 'explain' }, 'This trail has no steps yet.'));
  };
  main.append(h('div', { class: 'reader-head' }, h('h2', {}, t.title),
    h('button', { class: 'btn primary', type: 'button', disabled: !t.steps.length, onclick: guarded(async () => { draw(await api(`/trails/${t.id}/replay`)); say('Trail replayed against the current sources.'); }) }, icon('seal'), 'Replay and verify')), out);
  draw();
  return root;
}

function knot(s, i, replayed) {
  const el = h('li', { class: 'knot' + (s.changed ? ' changed' : '') },
    h('p', { class: 'claim' }, `${i + 1}. ${s.claim}`),
    h('p', { class: 'qt' }, '“' + s.quote + '”'),
    h('div', { class: 'facts' }, h('span', { class: 'pill' }, `${s.doc} · v${s.ver}`), h('span', { class: 'pill mono' }, 'receipt ' + s.receipt.slice(0, 10) + '…')));
  if (replayed) {
    const f = el.querySelector('.facts');
    f.append(s.sig_ok ? chip('verbatim', 'Signature holds') : chip('unsupported', 'Signature fails'));
    f.append(chip(s.then.verdict, `Then: ${V[s.then.verdict].label}`), chip(s.now.verdict, `Now: ${V[s.now.verdict].label}${s.now.severity ? ' · ' + s.now.severity : ''}`));
    if (s.changed) el.append(h('p', { class: 'alert' }, icon('wave'), ' The source has changed since this step was sealed.'));
    if (s.err) el.append(h('p', { class: 'err' }, s.err));
  }
  return el;
}

/* ---------------- ledger ---------------- */

const EVT = { 'doc.version': 'source version added', 'cite.add': 'citation recorded', 'receipt.issue': 'receipt sealed', 'trail.create': 'trail created', 'trail.step': 'trail step added', 'blob.redact': 'text redacted' };

async function viewLedger() {
  const [status, events] = await Promise.all([api('/status'), api('/events')]);
  const audit = h('div', { 'aria-live': 'polite' });
  const root = h('div', { class: 'view' },
    h('h1', { tabindex: '-1' }, 'Ledger'),
    h('p', { class: 'lede' }, 'Everything Revenant knows is an append-only log. Each entry carries the hash of the one before it, so changing or deleting any line breaks every line after it.'));
  const verify = guarded(async () => {
    const a = await api('/audit');
    audit.replaceChildren(a.ok ? h('p', { class: 'verdict-big tone-verbatim' }, icon('check'), `Chain intact: ${a.events} events`) : h('p', { class: 'verdict-big tone-unsupported' }, icon('x'), 'Chain broken'), a.ok ? null : h('p', { class: 'err' }, a.error));
  });
  root.append(h('div', { class: 'seance' },
    h('div', { class: 'stack' },
      h('section', { class: 'panel' }, h('h2', {}, 'The chain'),
        h('dl', { class: 'kv' }, h('dt', {}, 'events'), h('dd', {}, String(status.events)), h('dt', {}, 'head'), h('dd', {}, status.head), h('dt', {}, 'signing key'), h('dd', {}, status.key), h('dt', {}, 'public key'), h('dd', {}, status.pub)),
        h('div', { class: 'row sec' }, h('button', { class: 'btn primary', type: 'button', onclick: verify }, 'Verify the whole chain')), audit),
      h('section', { class: 'panel' }, h('h2', {}, 'Latest entries'),
        events.length ? h('ol', { class: 'chain', reversed: true }, events.slice().reverse().map(e => h('li', {}, h('span', { class: 'seq' }, '#' + e.seq), EVT[e.type] || e.type, h('code', {}, e.hash.slice(0, 12) + '…'), h('time', { datetime: e.ts, class: 'hint' }, when(e.ts))))) : h('p', { class: 'hint' }, 'The log is empty.'))),
    receiptChecker(status.pub)));
  return root;
}

function receiptChecker(pub) {
  const json = h('textarea', { class: 'mono', id: 'rc-json', placeholder: '{ "body": { … }, "sig": "…" }', spellcheck: 'false' });
  const src = h('textarea', { class: 'mono', id: 'rc-src', spellcheck: 'false' });
  const pin = h('input', { type: 'text', id: 'rc-pin', placeholder: 'base64 public key (optional)', autocomplete: 'off' });
  const out = h('div', { 'aria-live': 'polite' });
  const list = (cls, arr) => h('ul', { class: 'checks ' + cls }, arr.map(s => h('li', {}, icon(cls === 'no' ? 'question' : 'check'), s)));
  return h('section', { class: 'panel' }, h('h2', {}, 'Verify a receipt'),
    h('p', { class: 'explain' }, 'Works on any receipt, from anyone. The check needs no log and no server state: only the signature and the excerpts inside the receipt.'),
    h('label', { class: 'field' }, h('span', {}, 'Receipt JSON'), json),
    h('label', { class: 'field' }, h('span', {}, 'Source text (optional)'), src),
    h('label', { class: 'field' }, h('span', {}, 'Pin the signer (optional)'), pin),
    h('div', { class: 'row' },
      h('button', { class: 'btn primary', type: 'button', onclick: guarded(async () => {
        let receipt;
        try { receipt = JSON.parse(json.value); } catch { return say('That is not valid JSON.', true); }
        const body = { receipt };
        if (src.value) body.source = src.value;
        if (pin.value.trim()) body.pin = pin.value.trim();
        const r = await api('/receipts/verify', body);
        out.replaceChildren(
          r.ok ? h('p', { class: 'verdict-big tone-verbatim' }, icon('check'), 'This receipt holds up') : h('p', { class: 'verdict-big tone-unsupported' }, icon('x'), 'This receipt does not hold up'),
          r.ok ? null : h('p', { class: 'err' }, r.error),
          ...(r.report && r.report.checked && r.report.checked.length ? [h('h3', {}, 'Checked'), list('yes', r.report.checked)] : []),
          h('h3', {}, 'Not checked, and cannot be'), list('no', (r.report && r.report.not_checked) || []));
      }) }, 'Verify offline'),
      h('button', { class: 'btn quiet', type: 'button', onclick: () => { pin.value = pub; } }, 'Pin this server’s key')), out);
}

/* ---------------- router ---------------- */

const views = { seance: viewSeance, archive: viewArchive, trails: viewTrails, ledger: viewLedger };

async function route() {
  const [, name = 'seance', a, b] = location.hash.split('/');
  const view = views[name] || viewSeance;
  document.querySelectorAll('.tabs a').forEach(t => t.setAttribute('aria-current', t.dataset.v === (views[name] ? name : 'seance') ? 'page' : 'false'));
  const main = $('#main');
  $('#toast').replaceChildren(); // a message about the last page should not follow you to the next
  try {
    const el = await view(a && decodeURIComponent(a), b && parseInt(b, 10));
    main.replaceChildren(el);
  } catch (e) {
    main.replaceChildren(h('div', { class: 'view' }, h('h1', { tabindex: '-1' }, 'Something went wrong'), h('p', { class: 'err', role: 'alert' }, e.message)));
  }
  $('h1', main)?.focus({ preventScroll: true });
  window.scrollTo(0, 0);
}

addEventListener('hashchange', route);
refreshStatus();
route();
