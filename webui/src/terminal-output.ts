export function safeTerminalText(encoded: string): string {
  const binary = atob(encoded);
  const bytes = Uint8Array.from(binary, character => character.charCodeAt(0));
  const decoded = new TextDecoder("utf-8", {fatal: false}).decode(bytes);
  return stripTerminalControls(decoded);
}

export function stripTerminalControls(value: string): string {
  let result = "";
  for (let index = 0; index < value.length; index += 1) {
    const code = value.charCodeAt(index);
    if (code === 27) {
      const marker = value.charCodeAt(index + 1);
      if (marker === 91) {
        index += 2;
        while (index < value.length && (value.charCodeAt(index) < 64 || value.charCodeAt(index) > 126)) index += 1;
      } else if (marker === 93) {
        index += 2;
        while (index < value.length && value.charCodeAt(index) !== 7) {
          if (value.charCodeAt(index) === 27 && value.charCodeAt(index + 1) === 92) {
            index += 1;
            break;
          }
          index += 1;
        }
      } else {
        index += 1;
      }
      continue;
    }
    if ((code < 32 && code !== 9 && code !== 10 && code !== 13) || code === 127) continue;
    result += value[index];
  }
  return result;
}
