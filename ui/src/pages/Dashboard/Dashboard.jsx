import React from 'react';
import PropTypes from 'prop-types';
import { Link, withRouter } from 'react-router-dom';
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
  Button,
  Card,
  SetupBox,
  Modal,
  ModalHeader,
  ModalBody,
  ModalFooter,
} from '@kerberos-io/ui';
import './Dashboard.scss';
import ReactTooltip from 'react-tooltip';
import config from '../../config';

// eslint-disable-next-line react/prefer-stateless-function
class Dashboard extends React.Component {
  constructor() {
    super();
    this.state = {
      liveviewLoaded: false,
      open: false,
      currentRecording: '',
    };
  }

  componentDidMount() {
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
    const liveview = document.getElementsByClassName('videocard-video');
    if (liveview && liveview.length > 0) {
      liveview[0].remove();
    }
  }

  handleClose() {
    this.setState({
      open: false,
      currentRecording: '',
    });
  }

  getCurrentTimestamp() {
    return Math.round(Date.now() / 1000);
  }

  openModal(file) {
    this.setState({
      open: true,
      currentRecording: file,
    });
  }

  render() {
    const { dashboard } = this.props;
    const { liveviewLoaded, open, currentRecording } = this.state;

    // We check if the camera was getting a valid frame
    // during the last 5 seconds, otherwise we assume the camera is offline.
    const isCameraOnline =
      this.getCurrentTimestamp() - dashboard.cameraOnline < 15;

    // We check if a connection is made to Kerberos Hub, or if Offline mode
    // has been turned on.
    const cloudOnline = this.getCurrentTimestamp() - dashboard.cloudOnline < 30;
    let cloudConnection = 'Not connected';
    if (dashboard.offlineMode === 'true') {
      cloudConnection = 'Offline mode';
    } else {
      cloudConnection = cloudOnline ? 'Connected' : 'Not connected';
    }

    return (
      <div id="dashboard">
        <Breadcrumb
          title="Dashboard"
          level1="Overview of your video surveilance"
          level1Link=""
        >
          <Link to="/media">
            <Button label="Watch recordings" icon="media" type="default" />
          </Link>
          <Link to="/settings">
            <Button
              label="Configure"
              icon="preferences"
              type={isCameraOnline ? 'neutral' : 'default'}
            />
          </Link>
        </Breadcrumb>

        <div className="stats grid-container --four-columns">
          <KPI
            number={dashboard.days ? dashboard.days.length : 0}
            divider="0"
            footer="Number of days"
          />
          <KPI
            number={
              dashboard.numberOfRecordings ? dashboard.numberOfRecordings : 0
            }
            divider="0"
            footer="Total recordings"
          />

          <Link to="/settings">
            <Card
              title="IP Camera"
              subtitle={isCameraOnline ? 'Connected' : 'not connected'}
              footer="Camera"
              icon={isCameraOnline ? 'circle-check-big' : 'circle-cross-big'}
            />
          </Link>
          <Link to="/settings">
            <Card
              title="Kerberos Hub"
              subtitle={cloudConnection}
              footer="Cloud"
              icon={
                cloudOnline && dashboard.offlineMode !== 'true'
                  ? 'circle-check-big'
                  : 'circle-cross-big'
              }
            />
          </Link>
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
                {dashboard.latestEvents &&
                  dashboard.latestEvents.map((event) => (
                    <TableRow
                      key={event.timestamp}
                      id="cells1"
                      bodycells={[
                        <>
                          <div className="time">
                            <Ellipse status="success" />{' '}
                            <p data-tip="10m and 5s ago">{event.time}</p>
                          </div>
                        </>,
                        <>
                          <p
                            className="pointer"
                            onClick={() =>
                              this.openModal(`${config.URL}/file/${event.key}`)
                            }
                          >
                            Motion was detected
                          </p>
                        </>,
                        <>
                          <span className="version">{event.camera_name}</span>
                          &nbsp;
                          <Icon label="cameras" />
                        </>,
                      ]}
                    />
                  ))}
              </TableBody>
              {open && (
                <Modal>
                  <ModalHeader
                    title="View recording"
                    onClose={() => this.handleClose()}
                  />
                  <ModalBody>
                    <video controls autoPlay>
                      <source src={currentRecording} type="video/mp4" />
                    </video>
                  </ModalBody>
                  <ModalFooter
                    right={
                      <>
                        <a
                          href={currentRecording}
                          download="video"
                          target="_blank"
                          rel="noreferrer"
                        >
                          <Button
                            label="Download"
                            icon="download"
                            type="button"
                            buttonType="button"
                          />
                        </a>
                        <Button
                          label="Close"
                          icon="cross-circle"
                          type="button"
                          buttonType="button"
                          onClick={() => this.handleClose()}
                        />
                      </>
                    }
                  />
                </Modal>
              )}
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

const mapDispatchToProps = (/* dispatch , ownProps */) => ({});

Dashboard.propTypes = {
  dashboard: PropTypes.objectOf(PropTypes.object).isRequired,
};

export default withRouter(
  connect(mapStateToProps, mapDispatchToProps)(Dashboard)
);
