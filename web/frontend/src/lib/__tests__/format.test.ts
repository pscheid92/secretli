import { formatExpiry } from "../format";

describe("formatExpiry", () => {
  const now = new Date(2026, 9, 5, 19, 53);

  it("says today, tomorrow or the date, with the time", () => {
    expect(formatExpiry(new Date(2026, 9, 5, 23, 30).toISOString(), now)).toMatch(/^today at /);
    expect(formatExpiry(new Date(2026, 9, 6, 0, 10).toISOString(), now)).toMatch(/^tomorrow at /);
    expect(formatExpiry(new Date(2026, 9, 12, 19, 53).toISOString(), now)).toMatch(/^on .+ at /);
  });

  it("does not mistake the same time a month later for today", () => {
    expect(formatExpiry(new Date(2026, 10, 5, 19, 53).toISOString(), now)).toMatch(/^on /);
  });
});
