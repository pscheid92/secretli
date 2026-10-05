import {
  formatCode,
  parseCode,
  randomWords,
  TRANSFER_WORDS,
  transferPassword,
} from "../transferWords";

describe("transfer word list", () => {
  it("has 1,296 lowercase words with unique three-letter prefixes", () => {
    expect(TRANSFER_WORDS).toHaveLength(1296);
    expect(new Set(TRANSFER_WORDS).size).toBe(1296);
    expect(new Set(TRANSFER_WORDS.map((w) => w.slice(0, 3))).size).toBe(1296);
    for (const word of TRANSFER_WORDS) expect(word).toMatch(/^[a-z]{3,10}$/);
  });

  it("picks words from the list", () => {
    for (let i = 0; i < 20; i++) {
      for (const word of randomWords()) expect(TRANSFER_WORDS).toContain(word);
    }
  });
});

describe("parseCode", () => {
  it("reads a formatted code back", () => {
    expect(parseCode(formatCode(7, ["acid", "rocket"]))).toEqual({
      ok: true,
      nameplate: 7,
      words: ["acid", "rocket"],
    });
  });

  it.each([
    "7 acid rocket",
    "  7-ACID-Rocket  ",
    "7.acid.rocket",
    "7 - acid - rocket",
    "7-aci-roc",
    "7-acid-rock",
  ])("accepts %j", (input) => {
    expect(parseCode(input)).toEqual({ ok: true, nameplate: 7, words: ["acid", "rocket"] });
  });

  it.each([
    "",
    "acid-rocket",
    "7-acid",
    "7-acid-rocket-extra",
    "0-acid-rocket",
    "1000-acid-rocket",
    "x-acid-rocket",
  ])("rejects the format of %j", (input) => {
    expect(parseCode(input)).toEqual({ ok: false, error: "format" });
  });

  it("names a word that isn't on the list", () => {
    expect(parseCode("7-acid-rokcet")).toEqual({
      ok: false,
      error: "unknown-word",
      word: "rokcet",
    });
    expect(parseCode("7-ac-rocket")).toEqual({ ok: false, error: "unknown-word", word: "ac" });
  });
});

describe("transferPassword", () => {
  it("uses only the words, joined by a dash", () => {
    expect(new TextDecoder().decode(transferPassword(["acid", "rocket"]))).toBe("acid-rocket");
  });
});
