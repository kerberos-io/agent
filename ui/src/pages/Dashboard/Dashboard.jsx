import React from 'react';
import PropTypes from 'prop-types';
import { Link, withRouter } from 'react-router-dom';
import { withTranslation } from 'react-i18next';
import { send } from '@giantmachines/redux-websocket';
import { connect } from 'react-redux';
import { interval } from 'rxjs';
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

    const { connected } = this.props;
    if (connected === true) {
      const { dispatchSend } = this.props;
      const message = {
        message_type: 'stream-sd',
      };
      dispatchSend(message);
    }
  }

  componentDidUpdate(prevProps) {
    const { connected: connectedPrev } = prevProps;
    const { connected } = this.props;
    if (connectedPrev === false && connected === true) {
      const { dispatchSend } = this.props;
      const message = {
        message_type: 'stream-sd',
      };
      dispatchSend(message);

      const requestStreamInterval = interval(3000);
      this.requestStreamSubscription = requestStreamInterval.subscribe(() => {
        dispatchSend(message);
      });
    }
  }

  componentWillUnmount() {
    const liveview = document.getElementsByClassName('videocard-video');
    if (liveview && liveview.length > 0) {
      liveview[0].remove();
    }

    if (this.requestStreamSubscription) {
      this.requestStreamSubscription.unsubscribe();
    }
    const { dispatchSend } = this.props;
    const message = {
      message_type: 'stop-sd',
    };
    dispatchSend(message);
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
    const { dashboard, t, images } = this.props;
    const { liveviewLoaded, open, currentRecording } = this.state;

    // We check if the camera was getting a valid frame
    // during the last 5 seconds, otherwise we assume the camera is offline.
    const isCameraOnline =
      this.getCurrentTimestamp() - dashboard.cameraOnline < 15;

    // We check if a connection is made to Kerberos Hub, or if Offline mode
    // has been turned on.
    const cloudOnline = this.getCurrentTimestamp() - dashboard.cloudOnline < 30;
    let cloudConnection = t('dashboard.not_connected');
    if (dashboard.offlineMode === 'true') {
      cloudConnection = t('dashboard.offline_mode');
    } else {
      cloudConnection = cloudOnline
        ? t('dashboard.connected')
        : t('dashboard.not_connected');
    }

    return (
      <div id="dashboard">
        <Breadcrumb
          title={t('dashboard.title')}
          level1={t('dashboard.heading')}
          level1Link=""
        >
          <Link to="/media">
            <Button
              label={t('breadcrumb.watch_recordings')}
              icon="media"
              type="default"
            />
          </Link>
          <Link to="/settings">
            <Button
              label={t('breadcrumb.configure')}
              icon="preferences"
              type={isCameraOnline ? 'neutral' : 'default'}
            />
          </Link>
        </Breadcrumb>

        <div className="stats grid-container --four-columns">
          <KPI
            number={dashboard.days ? dashboard.days.length : 0}
            divider="0"
            footer={t('dashboard.number_of_days')}
          />
          <KPI
            number={
              dashboard.numberOfRecordings ? dashboard.numberOfRecordings : 0
            }
            divider="0"
            footer={t('dashboard.total_recordings')}
          />

          <Link to="/settings">
            <Card
              title="IP Camera"
              subtitle={
                isCameraOnline
                  ? t('dashboard.connected')
                  : t('dashboard.not_connected')
              }
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
            <h2>{t('dashboard.latest_events')}</h2>

            {(!dashboard.latestEvents ||
              dashboard.latestEvents.length === 0) && (
              <SetupBox
                dashed
                url="/settings"
                btnicon="preferences"
                btnlabel={t('dashboard.configure_connection')}
                header={t('dashboard.no_events')}
                text={t('dashboard.no_events_description')}
              />
            )}

            {dashboard.latestEvents && dashboard.latestEvents.length > 0 && (
              <Table>
                <TableHeader>
                  <TableRow
                    id="header"
                    headercells={[
                      t('dashboard.time'),
                      t('dashboard.description'),
                      t('dashboard.name'),
                    ]}
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
                            <div
                              className="time"
                              onClick={() =>
                                this.openModal(
                                  `${config.URL}/file/${event.key}`
                                )
                              }
                            >
                              <Ellipse status="success" />{' '}
                              <p data-tip="10m and 5s ago">{event.time}</p>
                            </div>
                          </>,
                          <>
                            <p
                              className="pointer event-description"
                              onClick={() =>
                                this.openModal(
                                  `${config.URL}/file/${event.key}`
                                )
                              }
                            >
                              {t('dashboard.motion_detected')}
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
            )}
          </div>
          <div>
            <h2>{t('dashboard.live_view')}</h2>
            {!liveviewLoaded && (
              <SetupBox
                btnicon="preferences"
                btnlabel={t('dashboard.configure_connection')}
                dashed
                url="/settings"
                header={t('dashboard.loading_live_view')}
                text={t('dashboard.loading_live_view_description')}
              />
            )}
            <div style={{ visibility: liveviewLoaded ? 'visible' : 'hidden' }}>
              <ImageCard
                imageSrc={`data:image/png;base64, ${
                  images.length ? images[0] : ''
                }`}
                onerror=""
              />
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
  connected: state.wss.connected,
  images: state.wss.images,
});

const mapDispatchToProps = (dispatch) => ({
  dispatchSend: (message) => dispatch(send(message)),
});

Dashboard.propTypes = {
  dashboard: PropTypes.object.isRequired,
  connected: PropTypes.bool.isRequired,
  images: PropTypes.array.isRequired,
  t: PropTypes.func.isRequired,
  dispatchSend: PropTypes.func.isRequired,
};

export default withTranslation()(
  withRouter(connect(mapStateToProps, mapDispatchToProps)(Dashboard))
);
