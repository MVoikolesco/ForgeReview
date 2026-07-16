export type ToastState = {
  show: boolean;
  message: string;
  tone: "success" | "error";
};
export function Toast({ state }: { state: ToastState }) {
  return state.show ? (
    <div className={`toast ${state.tone}`}>
      <span>{state.tone === "success" ? "✓" : "!"}</span>
      {state.message}
    </div>
  ) : null;
}
