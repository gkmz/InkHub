import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { SuggestionHistory } from "./SuggestionHistory";

test("建议历史展示版本摘要并以只读方式查看详情", async () => {
  const onSelect = vi.fn();
  render(<SuggestionHistory
    items={[{ id: "s2", generated_at: "2026-08-02T10:00:00Z", model: "test-model", input_content_hash: "hash-2", state: "pending", suggestion_count: 3, current: true }]}
    selected={{ id: "s2", generated_at: "2026-08-02T10:00:00Z", model: "test-model", input_content_hash: "hash-2", state: "pending", suggestions_stale: false, suggestions: [{ id: "d1", field: "description", name: "新的描述", value: "新的描述", reason: "", new_term: false, usage_count: 0 }] }}
    loading={false}
    detailLoading={false}
    error=""
    onSelect={onSelect}
    onRetry={vi.fn()}
    onClose={vi.fn()}
  />);
  expect(screen.getByRole("heading", { name: "建议历史" })).toBeInTheDocument();
  expect(screen.getByText(/test-model/)).toBeInTheDocument();
  expect(screen.getByText("新的描述")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: /采用/ })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: /当前/ }));
  expect(onSelect).toHaveBeenCalledWith("s2");
});
