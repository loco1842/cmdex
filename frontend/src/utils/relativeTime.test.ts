import { describe, expect, it } from "vitest";
import { formatRelativeTime } from "./relativeTime";

const NOW = Date.parse("2026-09-04T12:00:00Z");

describe("formatRelativeTime", () => {
  it("returns empty for missing or invalid input", () => {
    expect(formatRelativeTime(undefined, NOW)).toBe("");
    expect(formatRelativeTime("", NOW)).toBe("");
    expect(formatRelativeTime("not-a-date", NOW)).toBe("");
  });

  it("says just now under a minute", () => {
    expect(formatRelativeTime("2026-09-04T11:59:30Z", NOW)).toBe("just now");
  });

  it("pluralizes minutes and hours", () => {
    expect(formatRelativeTime("2026-09-04T11:59:00Z", NOW)).toBe("1 minute ago");
    expect(formatRelativeTime("2026-09-04T11:50:00Z", NOW)).toBe("10 minutes ago");
    expect(formatRelativeTime("2026-09-04T11:00:00Z", NOW)).toBe("1 hour ago");
    expect(formatRelativeTime("2026-09-04T10:00:00Z", NOW)).toBe("2 hours ago");
  });

  it("reports days under 30 days", () => {
    expect(formatRelativeTime("2026-09-03T12:00:00Z", NOW)).toBe("1 day ago");
    expect(formatRelativeTime("2026-08-30T12:00:00Z", NOW)).toBe("5 days ago");
  });

  it("falls back to a date for older stamps and ignores future ones", () => {
    expect(formatRelativeTime("2026-06-01T12:00:00Z", NOW)).not.toBe("");
    expect(formatRelativeTime("2026-09-04T12:00:01Z", NOW)).toBe("");
  });
});
