import React from 'react';
// @ts-ignore
import Drop from 'tether-drop';
import { GrafanaRouteComponentProps } from './types';
import { locationSearchToObject, navigationLogger } from '@grafana/runtime';
import { keybindingSrv } from '../services/keybindingSrv';
import { shouldReloadPage } from './utils';
import { analyticsService } from '../services/analytics';

export interface Props extends Omit<GrafanaRouteComponentProps, 'queryParams'> {}

export class GrafanaRoute extends React.Component<Props> {
  componentDidMount() {
    this.updateBodyClassNames();
    this.cleanupDOM();

    // unbinds all and re-bind global keybindins
    keybindingSrv.reset();
    keybindingSrv.initGlobals();
    analyticsService.track();
    delete (this.props.history.location.state as any)?.forceRouteReload;
    navigationLogger('GrafanaRoute', false, 'Mounted', this.props.match);
  }

  componentDidUpdate(prevProps: Props) {
    this.cleanupDOM();

    // Clear force reload state when route updates
    if (shouldReloadPage(this.props.location)) {
      navigationLogger('GrafanaRoute', false, 'Force reload', this.props, prevProps);
      delete (this.props.history.location.state as any)?.forceRouteReload;
    }

    analyticsService.track();
    navigationLogger('GrafanaRoute', false, 'Updated', this.props, prevProps);
  }

  componentWillUnmount() {
    this.updateBodyClassNames(true);
    navigationLogger('GrafanaRoute', false, 'Unmounted', this.props.route);
  }

  getPageClasses() {
    return this.props.route.pageClass ? this.props.route.pageClass.split(' ') : [];
  }

  updateBodyClassNames(clear = false) {
    for (const cls of this.getPageClasses()) {
      if (clear) {
        document.body.classList.remove(cls);
      } else {
        document.body.classList.add(cls);
      }
    }
  }

  cleanupDOM() {
    document.body.classList.remove('sidemenu-open--xs');

    // cleanup tooltips
    const tooltipById = document.getElementById('tooltip');
    tooltipById?.parentElement?.removeChild(tooltipById);

    const tooltipsByClass = document.querySelectorAll('.tooltip');
    for (let i = 0; i < tooltipsByClass.length; i++) {
      const tooltip = tooltipsByClass[i];
      tooltip.parentElement?.removeChild(tooltip);
    }

    // cleanup tether-drop
    for (const drop of Drop.drops) {
      drop.destroy();
    }
  }

  render() {
    const { props } = this;
    navigationLogger('GrafanaRoute', false, 'Rendered', props.route);

    const RouteComponent = props.route.component;

    return <RouteComponent {...props} queryParams={locationSearchToObject(props.location.search)} />;
  }
}
