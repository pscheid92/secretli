import { parseShareLink } from "../shareLink";

const ORIGIN = "https://secretli.example";
const SECRET = "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789-_AbCdE";
const TOKEN = "ZyXwVuTsRqPoNmLkJiHgFeDcBa9876543210_-ZyXwV";

describe("parseShareLink", () => {
  it("accepts a recipient link for this origin", () => {
    expect(parseShareLink(`${ORIGIN}/s#${SECRET}`, ORIGIN)).toEqual({
      kind: "share",
      fragment: SECRET,
    });
  });

  it("accepts an owner link and keeps the deletion token", () => {
    expect(parseShareLink(`${ORIGIN}/s#${SECRET}!${TOKEN}`, ORIGIN)).toEqual({
      kind: "share",
      fragment: `${SECRET}!${TOKEN}`,
    });
  });

  it("ignores surrounding whitespace", () => {
    expect(parseShareLink(`  ${ORIGIN}/s#${SECRET}\n`, ORIGIN).kind).toBe("share");
  });

  it("names the host of a share link for another origin", () => {
    expect(parseShareLink(`https://other.example:8443/s#${SECRET}`, ORIGIN)).toEqual({
      kind: "other-host",
      host: "other.example:8443",
    });
  });

  it("treats a different scheme as another origin", () => {
    expect(parseShareLink(`http://secretli.example/s#${SECRET}`, ORIGIN).kind).toBe("other-host");
  });

  it.each([
    ["plain text", "hello world"],
    ["an empty string", ""],
    ["a javascript: URL", `javascript:alert(1)//s#${SECRET}`],
    ["a data: URL", `data:text/html,<p>/s#${SECRET}</p>`],
    ["another path", `${ORIGIN}/share#${SECRET}`],
    ["a trailing slash on the path", `${ORIGIN}/s/#${SECRET}`],
    ["no fragment", `${ORIGIN}/s`],
    ["a key one character short", `${ORIGIN}/s#${SECRET.slice(1)}`],
    ["a key one character long", `${ORIGIN}/s#${SECRET}A`],
    ["standard base64 characters", `${ORIGIN}/s#${SECRET.slice(2)}+/`],
    ["a short deletion token", `${ORIGIN}/s#${SECRET}!${TOKEN.slice(1)}`],
    ["an empty deletion token", `${ORIGIN}/s#${SECRET}!`],
  ])("rejects %s", (_, text) => {
    expect(parseShareLink(text, ORIGIN)).toEqual({ kind: "invalid" });
  });

  it("rejects a malformed link for another origin without naming its host", () => {
    expect(parseShareLink("https://phish.example/login", ORIGIN)).toEqual({ kind: "invalid" });
  });
});
