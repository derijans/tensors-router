export function showStatus(element: HTMLElement, message: string, isError: boolean): void {
  element.textContent = message;
  element.classList.toggle("error-text", isError);
}
