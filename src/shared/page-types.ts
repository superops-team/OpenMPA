/** Document display shape shared by the Web and desktop views. */
export interface Page {
  id: string;
  spaceId: string;
  parentId: string | null;
  title: string;
  content: string;
  revision: number;
  createdAt: number;
  updatedAt: number;
  /** Legacy Web provenance; desktop source references are scoped separately. */
  sourceThreadId: string | null;
}
