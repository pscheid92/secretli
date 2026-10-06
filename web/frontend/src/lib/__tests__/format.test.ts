import { formatExpiry, formatMoment } from "../format";

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

describe("formatMoment", () => {
  // A Monday evening.
  const now = new Date(2026, 9, 5, 19, 53);

  it("says yesterday, then the weekday for the past week, then the date", () => {
    expect(formatMoment(new Date(2026, 9, 4, 8, 10).toISOString(), now)).toMatch(/^yesterday at /);
    expect(formatMoment(new Date(2026, 9, 1, 14, 2).toISOString(), now)).toMatch(
      /^on Thursday at /,
    );
    expect(formatMoment(new Date(2026, 8, 28, 14, 2).toISOString(), now)).toMatch(
      /^on .*Sep.* at /,
    );
  });

  it("adds the year when it is not this one", () => {
    expect(formatMoment(new Date(2025, 11, 31, 23, 59).toISOString(), now)).toMatch(/2025/);
  });
});
