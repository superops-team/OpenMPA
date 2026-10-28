export interface ImageDraft {
  runtimeId: string;
  runtimeVersion: string;
  runtimeImage: string;
  toolId: string;
  toolVersion: string;
  toolImage: string;
}
export class ImageReviewError extends Error {
  constructor(readonly field: string) {
    super(`${field} is required before review.`);
  }
}
export function reviewImages(draft: ImageDraft) {
  for (const [name, value] of Object.entries(draft))
    if (!value.trim()) throw new ImageReviewError(name);
  return Object.freeze({
    runtimeId: draft.runtimeId,
    expectedRuntimeVersion: draft.runtimeVersion,
    runtimeImage: draft.runtimeImage.trim(),
    toolId: draft.toolId,
    expectedToolVersion: draft.toolVersion,
    toolImage: draft.toolImage.trim(),
  });
}
