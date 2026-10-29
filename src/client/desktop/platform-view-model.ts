export const platformVM = (state: {
  signedIn: boolean;
  principal?: { accountId?: string; scope?: string } | null;
  region?: string;
  epoch?: number;
}) => ({
  signedIn: state.signedIn,
  accountId: state.principal?.accountId || '',
  region: state.region || 'cn-beijing',
  epoch: state.epoch,
  scope: state.principal?.scope,
});
