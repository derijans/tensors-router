const escapeCode = 0x1b;
const bellCode = 0x07;
const controlSequenceIntroducer = 0x5b;
const operatingSystemCommandIntroducer = 0x5d;
const backslashCode = 0x5c;
const deleteCode = 0x7f;
const firstPrintableCode = 0x20;
const preservedWhitespaceCodes = new Set([0x09, 0x0a, 0x0d]);

export function safeTerminalText(encoded: string): string {
  const binary = atob(encoded);
  const bytes = Uint8Array.from(binary, character => codeAt(character, 0));
  const decoded = new TextDecoder("utf-8", {fatal: false}).decode(bytes);
  return stripTerminalControls(decoded);
}

export function stripTerminalControls(value: string): string {
  let result = "";
  let index = 0;
  while (index < value.length) {
    const code = codeAt(value, index);
    if (code === escapeCode) {
      index = indexAfterEscapeSequence(value, index);
      continue;
    }
    if (isKeptCharacter(code)) {
      result += value[index];
    }
    index += 1;
  }
  return result;
}

function indexAfterEscapeSequence(value: string, escapeIndex: number): number {
  const marker = codeAt(value, escapeIndex + 1);
  if (marker === controlSequenceIntroducer) {
    return indexAfterControlSequence(value, escapeIndex + 2);
  }
  if (marker === operatingSystemCommandIntroducer) {
    return indexAfterOperatingSystemCommand(value, escapeIndex + 2);
  }
  return escapeIndex + 2;
}

function indexAfterControlSequence(value: string, index: number): number {
  while (index < value.length && !isControlSequenceFinalByte(codeAt(value, index))) {
    index += 1;
  }
  return index + 1;
}

function indexAfterOperatingSystemCommand(value: string, index: number): number {
  while (index < value.length) {
    const code = codeAt(value, index);
    if (code === bellCode) {
      return index + 1;
    }
    if (code === escapeCode && codeAt(value, index + 1) === backslashCode) {
      return index + 2;
    }
    index += 1;
  }
  return index + 1;
}

function isControlSequenceFinalByte(code: number): boolean {
  return code >= 0x40 && code <= 0x7e;
}

function isKeptCharacter(code: number): boolean {
  if (code === deleteCode) {
    return false;
  }
  return code >= firstPrintableCode || preservedWhitespaceCodes.has(code);
}

function codeAt(value: string, index: number): number {
  return value.codePointAt(index) ?? -1;
}
