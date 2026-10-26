export interface ImageDraft {
  runtimeId: string;
  runtimeVersion: string;
  runtimeImage: string;
  toolId: string;
  toolVersion: string;
  toolImage: string;
}
export function reviewImages(draft: ImageDraft) {
  for (const [name, value] of Object.entries(draft))
    if (!value.trim()) throw new Error(`${name} is required before review.`);
  return Object.freeze({
    runtimeId: draft.runtimeId,
    expectedRuntimeVersion: draft.runtimeVersion,
    runtimeImage: draft.runtimeImage.trim(),
    toolId: draft.toolId,
    expectedToolVersion: draft.toolVersion,
    toolImage: draft.toolImage.trim(),
  });
}
