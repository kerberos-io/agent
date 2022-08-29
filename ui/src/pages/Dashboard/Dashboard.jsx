import React from 'react';
import PropTypes from 'prop-types';
import { withRouter } from 'react-router-dom';
import { connect } from 'react-redux';
import {
  Breadcrumb,
  KPI,
  ImageCard,
  Table,
  TableHeader,
  TableBody,
  TableRow,
  Icon,
  Ellipse,
  Card,
  SetupBox,
} from '@kerberos-io/ui';
import './Dashboard.scss';
import ReactTooltip from 'react-tooltip';
import { interval } from 'rxjs';
import config from '../../config';
import { GetDashboardInformation } from '../../actions/agent';

// eslint-disable-next-line react/prefer-stateless-function
class Dashboard extends React.Component {
  constructor() {
    super();
    this.state = {
      liveviewLoaded: false,
    };
  }

  componentDidMount() {
    const { dispatchGetDashboardInformation } = this.props;
    dispatchGetDashboardInformation();

    const interval$ = interval(2000);
    this.subscription = interval$.subscribe(() => {
      dispatchGetDashboardInformation();
    });

    const liveview = document.getElementsByClassName('videocard-video');
    if (liveview && liveview.length > 0) {
      liveview[0].addEventListener('load', () => {
        this.setState({
          liveviewLoaded: true,
        });
      });
    }
  }

  componentWillUnmount() {
    this.subscription.unsubscribe();
  }

  getCurrentTimestamp() {
    return Math.round(Date.now() / 1000);
  }

  render() {
    const { dashboard } = this.props;
    const { liveviewLoaded } = this.state;
    const isCameraOnline =
      this.getCurrentTimestamp() - dashboard.cameraOnline < 5;
    return (
      <div>
        <Breadcrumb
          title="Dashboard"
          level1="Overview of your video surveilance"
          level1Link=""
        >
          {/* <Link to="/deployments">
            <Button
              label="Add Kerberos Agent"
              icon="plus-circle"
              type="default"
            />
    </Link> */}
        </Breadcrumb>

        <div className="stats grid-container --four-columns">
          <KPI number="69" divider="0" footer="Number of days" />
          <KPI
            number={
              dashboard.numberOfRecordings ? dashboard.numberOfRecordings : 0
            }
            divider="0"
            footer="Total recordings"
          />
          <Card
            title="Camera"
            subtitle={
              isCameraOnline ? 'succesfully connected' : 'not connected'
            }
            footer="IP Camera"
            icon={isCameraOnline ? 'circle-check-big' : 'circle-cross-big'}
          />
          <Card
            title="Cloud"
            subtitle="Not connected"
            footer="Kerberos Hub"
            icon="circle-cross-big"
          />
        </div>
        <hr />
        <div className="stats grid-container --two-columns">
          <div>
            <h2>Latest events</h2>
            <Table>
              <TableHeader>
                <TableRow
                  id="header"
                  headercells={['time', 'description', 'name']}
                />
              </TableHeader>
              <TableBody>
                <TableRow
                  id="cells1"
                  bodycells={[
                    <>
                      <Ellipse status="success" />{' '}
                      <p data-tip="10m and 5s ago">19:45:10</p>
                    </>,
                    <>
                      <p>Motion was detected</p>
                    </>,
                    <>
                      <span className="version">Frontdoor</span>&nbsp;
                      <Icon label="cameras" />
                    </>,
                  ]}
                />
                <TableRow
                  id="cells1"
                  bodycells={[
                    <>
                      <Ellipse status="success" />{' '}
                      <p data-tip="10m and 5s ago">18:23:44</p>
                    </>,
                    <>
                      <p>Motion was detected</p>
                    </>,
                    <>
                      <span>Frontdoor</span>&nbsp;
                      <Icon label="cameras" />
                    </>,
                  ]}
                />
                <TableRow
                  id="cells1"
                  bodycells={[
                    <>
                      <Ellipse status="success" />{' '}
                      <p data-tip="10m and 5s ago">18:20:29</p>
                    </>,
                    <>
                      <p>Motion was detected</p>
                    </>,
                    <>
                      <span className="version">Frontdoor</span>&nbsp;
                      <Icon label="cameras" />
                    </>,
                  ]}
                />
                <TableRow
                  id="cells1"
                  bodycells={[
                    <>
                      <Ellipse status="success" />{' '}
                      <p data-tip="10m and 5s ago">15:16:58</p>
                    </>,
                    <>
                      <p>Motion was detected</p>
                    </>,
                    <>
                      <span className="version">Frontdoor</span>&nbsp;
                      <Icon label="cameras" />
                    </>,
                  ]}
                />
                <TableRow
                  id="cells1"
                  bodycells={[
                    <>
                      <Ellipse status="success" />{' '}
                      <p data-tip="10m and 5s ago">10:05:44</p>
                    </>,
                    <>
                      <p>Motion was detected</p>
                    </>,
                    <>
                      <span className="version">Frontdoor</span>&nbsp;
                      <Icon label="cameras" />
                    </>,
                  ]}
                />
              </TableBody>
            </Table>
          </div>
          <div>
            <h2>Live view</h2>
            {!liveviewLoaded && (
              <SetupBox
                btnicon="cameras"
                btnlabel="Configure connection"
                dashed
                header="Loading live view"
                text="Hold on we are loading your live view here. If you didn't configure your camera connection, update it on the settings pages."
              />
            )}
            <div style={{ visibility: liveviewLoaded ? 'visible' : 'hidden' }}>
              <ImageCard imageSrc={`${config.API_URL}/stream?token=xxxx`} />
            </div>
          </div>
        </div>
        <ReactTooltip />
      </div>
    );
  }
}

const mapStateToProps = (state /* , ownProps */) => ({
  dashboard: state.agent.dashboard,
});

const mapDispatchToProps = (dispatch /* , ownProps */) => ({
  dispatchGetDashboardInformation: (dashboard, success, error) =>
    dispatch(GetDashboardInformation(dashboard, success, error)),
});

Dashboard.propTypes = {
  dashboard: PropTypes.objectOf(PropTypes.object).isRequired,
  dispatchGetDashboardInformation: PropTypes.func.isRequired,
};

export default withRouter(
  connect(mapStateToProps, mapDispatchToProps)(Dashboard)
);
