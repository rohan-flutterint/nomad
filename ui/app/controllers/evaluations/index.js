import Controller from '@ember/controller';
import { action } from '@ember/object';
import { tracked } from '@glimmer/tracking';
import { inject as service } from '@ember/service';

export default class EvaluationsController extends Controller {
  @service store;
  @service userSettings;

  queryParams = ['nextToken', 'pageSize', 'status', 'triggeredBy', 'namespace'];

  get shouldDisableNext() {
    return !this.model.meta?.nextToken;
  }

  get shouldDisablePrev() {
    return !this.previousTokens.length;
  }

  get optionsEvaluationsStatus() {
    return [
      { key: null, label: 'All' },
      { key: 'blocked', label: 'Blocked' },
      { key: 'pending', label: 'Pending' },
      { key: 'complete', label: 'Complete' },
      { key: 'failed', label: 'Failed' },
      { key: 'canceled', label: 'Canceled' },
    ];
  }

  get optionsTriggeredBy() {
    return [
      { key: null, label: 'All' },
      { key: 'job-register', label: 'Job Register' },
      { key: 'job-deregister', label: 'Job Deregister' },
      { key: 'periodic-job', label: 'Periodic Job' },
      { key: 'node-drain', label: 'Node Drain' },
      { key: 'node-update', label: 'Node Update' },
      { key: 'alloc-stop', label: 'Allocation Stop' },
      { key: 'scheduled', label: 'Scheduled' },
      { key: 'rolling-update', label: 'Rolling Update' },
      { key: 'deployment-watcher', label: 'Deployment Watcher' },
      { key: 'failed-follow-up', label: 'Failed Follow Up' },
      { key: 'max-plan-attempts', label: 'Max Plan Attempts' },
      { key: 'alloc-failure', label: 'Allocation Failure' },
      { key: 'queued-allocs', label: 'Queued Allocations' },
      { key: 'preemption', label: 'Preemption' },
      { key: 'job-scaling', label: 'Job Scalling' },
    ];
  }

  get optionsNamespaces() {
    const namespaces = this.store.peekAll('namespace').map((namespace) => ({
      key: namespace.name,
      label: namespace.name,
    }));

    // Create default namespace selection
    namespaces.unshift({
      key: null,
      label: 'All (*)',
    });

    return namespaces;
  }

  @tracked pageSize = this.userSettings.pageSize;
  @tracked nextToken = null;
  @tracked previousTokens = [];
  @tracked status = null;
  @tracked triggeredBy = null;
  @tracked namespace = '*';

  @action
  onChange(newPageSize) {
    this.pageSize = newPageSize;
  }

  @action
  onNext(nextToken) {
    this.previousTokens = [...this.previousTokens, this.nextToken];
    this.nextToken = nextToken;
  }

  @action
  onPrev() {
    const lastToken = this.previousTokens.pop();
    this.previousTokens = [...this.previousTokens];
    this.nextToken = lastToken;
  }

  @action
  refresh() {
    this._resetTokens();
    this.status = null;
    this.pageSize = this.userSettings.pageSize;
  }

  @action
  setQueryParam(qp, selection) {
    this._resetTokens();
    this[qp] = selection;
  }

  _resetTokens() {
    this.nextToken = null;
    this.previousTokens = [];
  }
}
