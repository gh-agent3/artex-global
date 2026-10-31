import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "./table";

const STORAGE_KEY = "test-table-column-widths";

function ExampleTable() {
  return (
    <Table resizeStorageKey={STORAGE_KEY} resizeLabel="Resize column">
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>Status</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableRow>
          <TableCell>Task</TableCell>
          <TableCell>Running</TableCell>
        </TableRow>
      </TableBody>
    </Table>
  );
}

function mockHeaderWidths(widths: number[]) {
  screen.getAllByRole("columnheader").forEach((header, index) => {
    vi.spyOn(header, "getBoundingClientRect").mockReturnValue({ width: widths[index] } as DOMRect);
  });
}

function mockPointerCapture(handle: HTMLElement) {
  Object.defineProperties(handle, {
    setPointerCapture: { value: vi.fn(), configurable: true },
    hasPointerCapture: { value: () => true, configurable: true },
    releasePointerCapture: { value: vi.fn(), configurable: true },
  });
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  window.localStorage.clear();
});

describe("resizable Table", () => {
  it("enforces the minimum width and restores saved widths", async () => {
    const { unmount } = render(<ExampleTable />);
    mockHeaderWidths([120, 100]);
    const handle = screen.getAllByRole("separator", { name: "Resize column" })[0];
    mockPointerCapture(handle);

    fireEvent.pointerDown(handle, { pointerId: 1, button: 0, clientX: 100 });
    fireEvent.pointerMove(handle, { pointerId: 1, clientX: 0 });
    expect(screen.getAllByRole("columnheader")[0]).toHaveStyle({ width: "48px" });

    fireEvent.pointerMove(handle, { pointerId: 1, clientX: 180 });
    fireEvent.pointerUp(handle, { pointerId: 1 });
    expect(screen.getAllByRole("columnheader")[0]).toHaveStyle({ width: "200px" });
    expect(JSON.parse(window.localStorage.getItem(STORAGE_KEY) ?? "{}")).toMatchObject({ "0": 200 });

    unmount();
    render(<ExampleTable />);
    await waitFor(() => expect(screen.getAllByRole("columnheader")[0]).toHaveStyle({ width: "200px" }));
  });

  it("resizes a focused separator with arrow keys", () => {
    render(<ExampleTable />);
    mockHeaderWidths([120, 100]);
    const handle = screen.getAllByRole("separator", { name: "Resize column" })[0];

    fireEvent.keyDown(handle, { key: "ArrowLeft" });

    expect(screen.getAllByRole("columnheader")[0]).toHaveStyle({ width: "112px" });
    expect(JSON.parse(window.localStorage.getItem(STORAGE_KEY) ?? "{}")).toMatchObject({ "0": 112 });
  });
});
