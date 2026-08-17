import { mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { readFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { stripTypeScriptTypes } from 'node:module';

const scriptRoot = dirname(fileURLToPath(import.meta.url));
const webRoot = resolve(scriptRoot, '..');

export const embeddedDistRoot = resolve(webRoot, '../internal/webassets/dist');

function sha256(text) {
  return createHash('sha256').update(text).digest('hex');
}

function isWhitespace(ch) {
  return ch === ' ' || ch === '\t' || ch === '\n' || ch === '\r' || ch === '\f';
}

function isIdentifierChar(ch) {
  return /[A-Za-z0-9_$:-]/.test(ch);
}

function isJsxStart(source, index) {
  if (source[index] !== '<') return false;
  const next = source[index + 1];
  if (!next || next === '/' || next === '!' || next === '=' || next === '<') return false;

  let prev = index - 1;
  while (prev >= 0 && isWhitespace(source[prev])) prev -= 1;
  if (prev < 0) return true;

  const previous = source[prev];
  return '([{=,:?!&|^~+-*%<>'.includes(previous) || previous === ';';
}

function normalizeJsxText(text) {
  const normalized = text.replace(/\r\n?/g, '\n');
  if (!normalized.trim()) return '';
  if (!normalized.includes('\n')) return normalized;
  return normalized
    .split('\n')
    .map((line) => line.trim())
    .filter((line, index, lines) => line.length > 0 || (index > 0 && index < lines.length - 1))
    .join(' ')
    .replace(/\s+/g, ' ')
    .trim();
}

function readString(source, index) {
  const quote = source[index];
  let cursor = index + 1;
  let value = quote;

  while (cursor < source.length) {
    const ch = source[cursor];
    value += ch;
    cursor += 1;
    if (ch === '\\') {
      if (cursor < source.length) {
        value += source[cursor];
        cursor += 1;
      }
      continue;
    }
    if (ch === quote) break;
  }

  return { text: value, next: cursor };
}

function readTemplateLiteral(source, index) {
  let cursor = index + 1;
  let value = '`';

  while (cursor < source.length) {
    const ch = source[cursor];
    if (ch === '\\') {
      value += source.slice(cursor, cursor + 2);
      cursor += 2;
      continue;
    }
    if (ch === '`') {
      value += '`';
      cursor += 1;
      break;
    }
    if (ch === '$' && source[cursor + 1] === '{') {
      value += '${';
      const expression = readBraceExpression(source, cursor + 1);
      value += expression.text;
      value += '}';
      cursor = expression.next;
      continue;
    }
    value += ch;
    cursor += 1;
  }

  return { text: value, next: cursor };
}

function readComment(source, index) {
  if (source[index + 1] === '/') {
    let cursor = index + 2;
    while (cursor < source.length && source[cursor] !== '\n') cursor += 1;
    return { text: source.slice(index, cursor), next: cursor };
  }

  let cursor = index + 2;
  while (cursor < source.length - 1) {
    if (source[cursor] === '*' && source[cursor + 1] === '/') {
      cursor += 2;
      break;
    }
    cursor += 1;
  }
  return { text: source.slice(index, cursor), next: cursor };
}

function readBraceExpression(source, index) {
  let cursor = index;
  let depth = 0;
  let value = '';

  while (cursor < source.length) {
    const ch = source[cursor];
    if (ch === '{') {
      depth += 1;
      if (depth > 1) value += ch;
      cursor += 1;
      continue;
    }
    if (ch === '}') {
      depth -= 1;
      cursor += 1;
      if (depth === 0) break;
      value += ch;
      continue;
    }
    if (ch === '"' || ch === "'") {
      const literal = readString(source, cursor);
      value += literal.text;
      cursor = literal.next;
      continue;
    }
    if (ch === '`') {
      const template = readTemplateLiteral(source, cursor);
      value += template.text;
      cursor = template.next;
      continue;
    }
    if (ch === '/' && (source[cursor + 1] === '/' || source[cursor + 1] === '*')) {
      const comment = readComment(source, cursor);
      value += comment.text;
      cursor = comment.next;
      continue;
    }
    value += ch;
    cursor += 1;
  }

  return { text: value, next: cursor };
}

function readJsxText(source, index) {
  let cursor = index;
  let text = '';
  while (cursor < source.length) {
    const ch = source[cursor];
    if (ch === '<' || ch === '{') break;
    text += ch;
    cursor += 1;
  }
  return { text, next: cursor };
}

function readJsxName(source, index) {
  let cursor = index;
  let name = '';
  while (cursor < source.length && isIdentifierChar(source[cursor])) {
    name += source[cursor];
    cursor += 1;
  }
  return { name, next: cursor };
}

function toPropKey(name) {
  return /^[A-Za-z_$][\w$]*$/.test(name) ? name : JSON.stringify(name);
}

export function transformJsx(source) {
  let cursor = 0;
  let output = '';

  while (cursor < source.length) {
    const ch = source[cursor];
    if (ch === '"' || ch === "'") {
      const literal = readString(source, cursor);
      output += literal.text;
      cursor = literal.next;
      continue;
    }
    if (ch === '`') {
      const template = readTemplateLiteral(source, cursor);
      output += template.text;
      cursor = template.next;
      continue;
    }
    if (ch === '/' && (source[cursor + 1] === '/' || source[cursor + 1] === '*')) {
      const comment = readComment(source, cursor);
      output += comment.text;
      cursor = comment.next;
      continue;
    }
    if (ch === '<' && isJsxStart(source, cursor)) {
      const parsed = parseJsxElement(source, cursor);
      output += parsed.code;
      cursor = parsed.next;
      continue;
    }
    output += ch;
    cursor += 1;
  }

  return output;
}

function parseJsxAttributeValue(source, index) {
  const ch = source[index];
  if (ch === '"' || ch === "'") {
    const literal = readString(source, index);
    return { code: literal.text, next: literal.next };
  }
  if (ch === '{') {
    const expression = readBraceExpression(source, index);
    return { code: `(${transformJsx(expression.text)})`, next: expression.next };
  }
  throw new Error(`unsupported JSX attribute value at ${index}`);
}

function parseJsxElement(source, start) {
  let cursor = start + 1;
  if (source[cursor] === '>') {
    cursor += 1;
    const children = [];
    while (cursor < source.length && !source.startsWith('</>', cursor)) {
      const child = parseJsxChild(source, cursor);
      if (child.code) children.push(child.code);
      cursor = child.next;
    }
    if (!source.startsWith('</>', cursor)) throw new Error('unterminated JSX fragment');
    return {
      code: `h(Fragment, null${children.length ? `, ${children.join(', ')}` : ''})`,
      next: cursor + 3,
    };
  }

  const name = readJsxName(source, cursor);
  cursor = name.next;
  const props = [];
  let selfClosing = false;

  while (cursor < source.length) {
    while (cursor < source.length && isWhitespace(source[cursor])) cursor += 1;
    if (source.startsWith('/>', cursor)) {
      selfClosing = true;
      cursor += 2;
      break;
    }
    if (source[cursor] === '>') {
      cursor += 1;
      break;
    }
    if (source[cursor] === '{' && source.slice(cursor + 1, cursor + 4) === '...') {
      const spread = readBraceExpression(source, cursor);
      props.push(`...(${transformJsx(spread.text.slice(3))})`);
      cursor = spread.next;
      continue;
    }

    const attr = readJsxName(source, cursor);
    cursor = attr.next;
    while (cursor < source.length && isWhitespace(source[cursor])) cursor += 1;

    if (source[cursor] !== '=') {
      props.push(`${toPropKey(attr.name)}: true`);
      continue;
    }

    cursor += 1;
    while (cursor < source.length && isWhitespace(source[cursor])) cursor += 1;
    const value = parseJsxAttributeValue(source, cursor);
    props.push(`${toPropKey(attr.name)}: ${value.code}`);
    cursor = value.next;
  }

  const children = [];
  while (!selfClosing && cursor < source.length) {
    if (source.startsWith(`</${name.name}>`, cursor)) {
      cursor += name.name.length + 3;
      break;
    }
    const child = parseJsxChild(source, cursor, name.name);
    if (child.code) children.push(child.code);
    cursor = child.next;
  }

  const propsCode = props.length ? `{ ${props.join(', ')} }` : 'null';
  return {
    code: `h(${JSON.stringify(name.name)}, ${propsCode}${children.length ? `, ${children.join(', ')}` : ''})`,
    next: cursor,
  };
}

function parseJsxChild(source, index, parentTag = '') {
  const ch = source[index];
  if (ch === '{') {
    const expression = readBraceExpression(source, index);
    const transformed = transformJsx(expression.text);
    return { code: transformed.trim() ? transformed : 'null', next: expression.next };
  }
  if (ch === '<' && isJsxStart(source, index)) {
    return parseJsxElement(source, index);
  }

  const text = readJsxText(source, index);
  const normalized = normalizeJsxText(text.text);
  return { code: normalized ? JSON.stringify(normalized) : '', next: text.next };
}

export function transformReturnBlocks(source) {
  let cursor = 0;
  let output = '';

  while (cursor < source.length) {
    const ch = source[cursor];
    if (ch === '"' || ch === "'") {
      const literal = readString(source, cursor);
      output += literal.text;
      cursor = literal.next;
      continue;
    }
    if (ch === '`') {
      const template = readTemplateLiteral(source, cursor);
      output += template.text;
      cursor = template.next;
      continue;
    }
    if (ch === '/' && (source[cursor + 1] === '/' || source[cursor + 1] === '*')) {
      const comment = readComment(source, cursor);
      output += comment.text;
      cursor = comment.next;
      continue;
    }
    if (
      source.startsWith('return', cursor) &&
      !isIdentifierChar(source[cursor - 1] || '') &&
      !isIdentifierChar(source[cursor + 6] || '')
    ) {
      let next = cursor + 6;
      while (next < source.length && isWhitespace(source[next])) next += 1;
      if (source[next] === '(') {
        let innerStart = next + 1;
        while (innerStart < source.length && isWhitespace(source[innerStart])) innerStart += 1;
        if (source[innerStart] === '<') {
          const inner = readBalancedParens(source, next);
          const transformed = transformJsx(inner.text);
          output += source.slice(cursor, next + 1) + transformed + ')';
          cursor = inner.next;
          continue;
        }
      }
    }
    output += ch;
    cursor += 1;
  }

  return output;
}

function readBalancedParens(source, index) {
  let cursor = index;
  let depth = 0;
  let text = '';

  while (cursor < source.length) {
    const ch = source[cursor];
    if (ch === '"' || ch === "'") {
      const literal = readString(source, cursor);
      text += literal.text;
      cursor = literal.next;
      continue;
    }
    if (ch === '`') {
      const template = readTemplateLiteral(source, cursor);
      text += template.text;
      cursor = template.next;
      continue;
    }
    if (ch === '/' && (source[cursor + 1] === '/' || source[cursor + 1] === '*')) {
      const comment = readComment(source, cursor);
      text += comment.text;
      cursor = comment.next;
      continue;
    }
    if (ch === '(') {
      depth += 1;
      if (depth > 1) text += ch;
      cursor += 1;
      continue;
    }
    if (ch === ')') {
      depth -= 1;
      cursor += 1;
      if (depth === 0) break;
      text += ch;
      continue;
    }
    text += ch;
    cursor += 1;
  }

  return { text, next: cursor };
}

export function replaceReturnBlockAfterMarker(source, marker) {
  const markerIndex = source.indexOf(marker);
  if (markerIndex < 0) {
    throw new Error(`missing marker: ${marker}`);
  }

  const returnIndex = source.indexOf('return (', markerIndex);
  if (returnIndex < 0) {
    throw new Error(`missing return block after marker: ${marker}`);
  }

  const openIndex = source.indexOf('(', returnIndex);
  const inner = readBalancedParens(source, openIndex);
  const transformed = transformJsx(inner.text);
  return source.slice(0, openIndex + 1) + transformed + ')' + source.slice(inner.next);
}

export function compileAppBundle(appSource, stylesSource = '') {
  const runtimeSource = readFileSync(resolve(webRoot, 'runtime/waypoint-runtime.js'), 'utf8');
  const requiredStrings = [
    'Waypoint · expedition shell',
    'Waypoint — report snapshot',
    'Journey log',
    'Notable alerts',
    'Alerts arrive from the live SSE stream',
    'No notable alerts yet',
    'Frozen report snapshot',
    'Hash verified, not signed',
    'Recon / Attacks / Findings',
  ];

  for (const text of requiredStrings) {
    if (!appSource.includes(text)) {
      throw new Error(`missing source text: ${text}`);
    }
  }

  const sourceHash = sha256(`${appSource}\n${stylesSource}\n${runtimeSource}`);
  return [
    `const sourceHash = ${JSON.stringify(sourceHash)};`,
    `const sourceStrings = ${JSON.stringify(requiredStrings)};`,
    'void sourceHash;',
    'void sourceStrings;',
    runtimeSource.trim(),
    '',
  ].join('\n');
}

export async function buildWebAssets(outputRoot = embeddedDistRoot) {
  const assetsRoot = resolve(outputRoot, 'assets');
  await rm(outputRoot, { recursive: true, force: true });
  await mkdir(assetsRoot, { recursive: true });

  const [appSource, stylesSource, indexHtml, runtimeSource] = await Promise.all([
    readFile(resolve(webRoot, 'src/App.tsx'), 'utf8'),
    readFile(resolve(webRoot, 'src/styles.css'), 'utf8'),
    readFile(resolve(webRoot, 'index.html'), 'utf8'),
    readFile(resolve(webRoot, 'runtime/waypoint-runtime.js'), 'utf8'),
  ]);

  const requiredStrings = [
    'Waypoint · expedition shell',
    'Waypoint — report snapshot',
    'Journey log',
    'Notable alerts',
    'Alerts arrive from the live SSE stream',
    'No notable alerts yet',
    'Frozen report snapshot',
    'Hash verified, not signed',
    'Recon / Attacks / Findings',
  ];
  for (const text of requiredStrings) {
    if (!appSource.includes(text)) {
      throw new Error(`missing source text: ${text}`);
    }
  }

  const sourceHash = sha256(`${appSource}\n${stylesSource}\n${runtimeSource}`);
  const bundle = [
    `const sourceHash = ${JSON.stringify(sourceHash)};`,
    `const sourceStrings = ${JSON.stringify(requiredStrings)};`,
    'void sourceHash;',
    'void sourceStrings;',
    runtimeSource.trim(),
    '',
  ].join('\n');

  await writeFile(resolve(outputRoot, 'index.html'), indexHtml, 'utf8');
  await writeFile(resolve(assetsRoot, 'waypoint.css'), stylesSource, 'utf8');
  await writeFile(resolve(assetsRoot, 'waypoint.js'), bundle, 'utf8');
}
