import { render, screen, waitFor } from "@testing-library/react";
import BuildVersion from "../BuildVersion";

const COMMIT = "860030f3e948ed3644c2498022d613e122796147";

function serveVersion(response: Response | Error) {
  const fetchMock = vi.spyOn(globalThis, "fetch");
  if (response instanceof Error) fetchMock.mockRejectedValue(response);
  else fetchMock.mockResolvedValue(response);
  return fetchMock;
}

describe("BuildVersion", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("shows the short commit and links to it", async () => {
    const fetchMock = serveVersion(new Response(JSON.stringify({ version: COMMIT })));

    render(<BuildVersion />);

    const link = await screen.findByRole("link", { name: "Build 860030f" });
    expect(link.getAttribute("href")).toBe(
      `https://github.com/pscheid92/secretli/commit/${COMMIT}`,
    );
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/version", expect.anything());
  });

  it("shows a local build without a link", async () => {
    serveVersion(new Response(JSON.stringify({ version: "dev" })));

    render(<BuildVersion />);

    await screen.findByText("Build dev");
    expect(screen.queryByRole("link")).toBeNull();
  });

  it("renders nothing when the version can't be read", async () => {
    const fetchMock = serveVersion(new TypeError("offline"));

    const { container } = render(<BuildVersion />);

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(container.textContent).toBe("");
  });
});
