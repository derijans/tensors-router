export function gpuOptionKey(key: string): boolean {
  const lowered = String(key).toLowerCase();
  return ["gpulayers", "tensor_split", "maingpu", "usecuda", "usecublas", "embeddingsgpu", "sdclipgpu", "sdflashattention"].includes(key) ||
    lowered.includes("gpu") ||
    lowered.includes("cuda");
}
