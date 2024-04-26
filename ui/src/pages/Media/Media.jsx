import React from 'react';
import PropTypes from 'prop-types';
import { withTranslation } from 'react-i18next';
import {
  Breadcrumb,
  VideoCard,
  Button,
  Modal,
  ModalHeader,
  ModalBody,
  ModalFooter,
  ControlBar,
  Tabs,
  Tab,
  Icon,
} from '@kerberos-io/ui';
import { Link, withRouter } from 'react-router-dom';
import { connect } from 'react-redux';
import { getEvents } from '../../actions/agent';
import config from '../../config';
import './Media.scss';
import TimePicker from '../../components/TimePicker/TimePicker';

// eslint-disable-next-line react/prefer-stateless-function
class Media extends React.Component {
  constructor() {
    super();
    this.state = {
      timestamp_offset_start: 0,
      timestamp_offset_end: 0,
      number_of_elements: 12,
      isScrolling: false,
      open: false,
      currentRecording: '',
    };
  }

  componentDidMount() {
    const { dispatchGetEvents } = this.props;
    dispatchGetEvents(this.state);
    document.addEventListener('scroll', this.trackScrolling);
  }

  componentWillUnmount() {
    document.removeEventListener('scroll', this.trackScrolling);
  }

  handleClose() {
    this.setState({
      open: false,
      currentRecording: '',
    });
  }
  
  trackScrolling = () => {
    const { events, dispatchGetEvents } = this.props;
    const { isScrolling } = this.state;
    const wrappedElement = document.getElementById('loader');
    if (!isScrolling && this.isBottom(wrappedElement)) {
      this.setState({
        isScrolling: true,
      });
      // Get last element
      const lastElement = events[events.length - 1];
      if (lastElement) {
        this.setState({
          timestamp_offset_end: parseInt(lastElement.timestamp, 10),
        });
        dispatchGetEvents(this.state, () => {
          setTimeout(() => {
            this.setState({
              isScrolling: false,
            });
          }, 1000);
        });
      }
    }
  };

  isBottom(el) {
    return el.getBoundingClientRect().bottom + 50 <= window.innerHeight;
  }
  handleChange(){
  
  }
  openModal(file) {
    this.setState({
      open: true,
      currentRecording: file,
    });
  }

  render() {
    const { events, eventsLoaded, t } = this.props;
    const { isScrolling, open, currentRecording } = this.state;
    return (
      <div id="media">
        <Breadcrumb
          title={t('recordings.title')}
          level1={t('recordings.heading')}
          level1Link=""
        >
          <Link to="/settings">
            <Button
              label={t('breadcrumb.configure')}
              icon="preferences"
              type="default"
            />
          </Link>
        </Breadcrumb>
        <ControlBar type="row">
          <Tabs>
            <TimePicker>
              onClick ={()=>console.log(this.state.date)};
            </TimePicker>
            <Tab
              label={t('settings.submenu.all')}
              value="all"
              onClick={() => this.changeTab('https://twitter.com')}
              icon={<Icon label="twitter" />}
            />
          </Tabs>
        </ControlBar>

        <div className="stats grid-container --four-columns">
          {events.map((event) => (
            <div
              key={event.key}
              onClick={() => this.openModal(`${config.URL}/file/${event.key}`)}
            >
              <VideoCard
                isMediaWall
                videoSrc={`${config.URL}/file/${event.key}`}
                hours={event.time}
                month={event.short_day}
                videoStatus=""
                duration=""
                headerStatus=""
                headerStatusTitle=""
                handleClickHD={() => true}
                handleClickSD={() => true}
              />
            </div>
          ))}
        </div>
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

        {!isScrolling && eventsLoaded !== 0 && (
          <div id="loader">
            <div className="lds-ripple">
              <div />
              <div />
            </div>
          </div>
        )}
      </div>
    );
  }
}

const mapStateToProps = (state /* , ownProps */) => ({
  events: state.agent.events,
  eventsLoaded: state.agent.eventsLoaded,
});

const mapDispatchToProps = (dispatch) => ({
  dispatchGetEvents: (eventFilter, success, error) =>
    dispatch(getEvents(eventFilter, success, error)),
});

Media.propTypes = {
  t: PropTypes.func.isRequired,
  events: PropTypes.objectOf(PropTypes.object).isRequired,
  eventsLoaded: PropTypes.number.isRequired,
  dispatchGetEvents: PropTypes.func.isRequired,
};

export default withTranslation()(
  withRouter(connect(mapStateToProps, mapDispatchToProps)(Media))
);
