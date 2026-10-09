import { describe, expect, it } from "vitest";
import { safeTerminalText, stripTerminalControls } from "../terminal-output";

describe("stripTerminalControls", () => {
  it("removes CSI colour and cursor sequences", () => {
    expect(stripTerminalControls("\x1b[1;31merror\x1b[0m done\x1b[2K")).toBe("error done");
  });

  it("removes OSC titles ended by BEL or by ESC backslash", () => {
    expect(stripTerminalControls("a\x1b]0;title\x07b")).toBe("ab");
    expect(stripTerminalControls("a\x1b]8;;https://x\x1b\\b")).toBe("ab");
  });

  it("drops a lone escape together with the character after it", () => {
    expect(stripTerminalControls("a\x1bcb")).toBe("ab");
  });

  it("drops an unterminated sequence through the end of the text", () => {
    expect(stripTerminalControls("kept\x1b[12;")).toBe("kept");
    expect(stripTerminalControls("kept\x1b]title")).toBe("kept");
  });

  it("keeps tab, newline and carriage return but drops other control characters and DEL", () => {
    expect(stripTerminalControls("a\tb\nc\rd\x00e\x08f\x7fg")).toBe("a\tb\nc\rdefg");
  });

  it("keeps characters outside the basic multilingual plane", () => {
    expect(stripTerminalControls("ok 😀 é")).toBe("ok 😀 é");
  });
});

describe("safeTerminalText", () => {
  it("decodes base64 UTF-8 and strips terminal controls", () => {
    const encoded = btoa(String.fromCodePoint(...new TextEncoder().encode("\x1b[32mgrün\x1b[0m")));
    expect(safeTerminalText(encoded)).toBe("grün");
  });
});
