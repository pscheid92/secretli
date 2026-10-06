import { takeSharedText } from "../shareTarget";

function visit(search: string) {
  window.history.replaceState(null, "", `/share${search}`);
}

describe("takeSharedText", () => {
  afterEach(() => visit(""));

  it("reads shared text and takes it out of the address bar", () => {
    visit("?title=Note&text=hunter2");

    expect(takeSharedText()).toBe("hunter2");
    expect(window.location.search).toBe("");
    expect(window.location.pathname).toBe("/share");
  });

  it("keeps a shared page's address under its text, once", () => {
    visit("?text=Look&url=https%3A%2F%2Fexample.org");
    expect(takeSharedText()).toBe("Look\nhttps://example.org");

    visit("?text=See%20https%3A%2F%2Fexample.org&url=https%3A%2F%2Fexample.org");
    expect(takeSharedText()).toBe("See https://example.org");

    visit("?url=https%3A%2F%2Fexample.org");
    expect(takeSharedText()).toBe("https://example.org");
  });

  it("falls back to the title when that is all an app sends", () => {
    visit("?title=Just%20a%20title");

    expect(takeSharedText()).toBe("Just a title");
    expect(window.location.search).toBe("");
  });

  it("leaves an ordinary visit alone", () => {
    visit("");

    expect(takeSharedText()).toBe("");
  });
});
